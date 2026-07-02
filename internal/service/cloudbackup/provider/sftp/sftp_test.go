package sftp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
	pkgsftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// unguardedDial dials without the production SSRF guard so protocol tests can
// reach the loopback test SSH server. SSRF-blocking behavior of the default
// (guarded) dialer is covered separately in sftp_ssrf_test.go.
func unguardedDial(ctx context.Context, network, addr string) (net.Conn, error) {
	return (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, network, addr)
}

// newTestProvider returns an SFTP provider that bypasses the SSRF guard, for
// use with loopback test servers.
func newTestProvider() *Provider {
	return NewWithDialer(unguardedDial, 0)
}

// fakeSSHServer is an in-process SSH+SFTP server backed by pkg/sftp's
// in-memory handlers. It serves a single ed25519 host key whose fingerprint
// is exposed for tests that want to assert host-key verification.
type fakeSSHServer struct {
	listener   net.Listener
	hostKeyFP  string
	username   string
	password   string
	rejectAuth bool
	rootDir    string
	closed     chan struct{}
}

func startFakeSSH(t *testing.T) *fakeSSHServer {
	t.Helper()

	// Generate ed25519 host key for the test server.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("ssh signer: %v", err)
	}

	srv := &fakeSSHServer{
		hostKeyFP: ssh.FingerprintSHA256(signer.PublicKey()),
		username:  "alice",
		password:  "secret",
		rootDir:   t.TempDir(),
		closed:    make(chan struct{}),
	}

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if srv.rejectAuth {
				return nil, errors.New("rejected for test")
			}
			if c.User() != srv.username || string(pass) != srv.password {
				return nil, errors.New("invalid credentials")
			}
			return nil, nil
		},
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv.listener = ln

	go srv.acceptLoop(cfg)

	t.Cleanup(func() {
		close(srv.closed)
		_ = ln.Close()
	})
	return srv
}

func (s *fakeSSHServer) addr() string {
	return s.listener.Addr().String()
}

func (s *fakeSSHServer) hostPort() (string, int) {
	h, p, _ := net.SplitHostPort(s.addr())
	port, _ := strconv.Atoi(p)
	return h, port
}

func (s *fakeSSHServer) acceptLoop(cfg *ssh.ServerConfig) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return
			default:
				return
			}
		}
		go s.handleConn(conn, cfg)
	}
}

func (s *fakeSSHServer) handleConn(c net.Conn, cfg *ssh.ServerConfig) {
	defer c.Close()
	_, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			return
		}
		go func(in <-chan *ssh.Request) {
			for req := range in {
				ok := false
				if req.Type == "subsystem" && len(req.Payload) >= 4 {
					name := string(req.Payload[4:])
					if name == "sftp" {
						ok = true
					}
				}
				_ = req.Reply(ok, nil)
			}
		}(requests)

		server, err := pkgsftp.NewServer(channel, pkgsftp.WithServerWorkingDirectory(s.rootDir))
		if err != nil {
			return
		}
		go func() {
			_ = server.Serve()
			_ = server.Close()
		}()
	}
}

func providerCfg(host string, port int, hostKeyFP string) provider.Config {
	return provider.Config{
		"host":                 host,
		"port":                 strconv.Itoa(port),
		"path":                 "ninerlog-test/",
		"host_key_fingerprint": hostKeyFP,
	}
}

func providerCreds() provider.Credentials {
	return provider.Credentials{
		"username": "alice",
		"password": "secret",
	}
}

// Tests --------------------------------------------------------------------

func TestProviderMetadata(t *testing.T) {
	p := newTestProvider()
	if p.Name() != "sftp" {
		t.Errorf("Name: %s", p.Name())
	}
	if p.DisplayName() == "" || p.Description() == "" {
		t.Errorf("missing display name/description")
	}
	if len(p.ConfigSchema()) == 0 || len(p.CredentialSchema()) == 0 {
		t.Errorf("missing schemas")
	}
	for _, f := range p.CredentialSchema() {
		if !f.Sensitive {
			t.Errorf("credential field %q not marked sensitive", f.Name)
		}
	}
}

func TestValidateSuccess(t *testing.T) {
	srv := startFakeSSH(t)
	host, port := srv.hostPort()
	p := newTestProvider()
	if err := p.Validate(context.Background(), providerCfg(host, port, srv.hostKeyFP), providerCreds()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateBadPassword(t *testing.T) {
	srv := startFakeSSH(t)
	host, port := srv.hostPort()
	p := newTestProvider()
	creds := provider.Credentials{"username": "alice", "password": "wrong"}
	err := p.Validate(context.Background(), providerCfg(host, port, srv.hostKeyFP), creds)
	if !errors.Is(err, provider.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestValidateHostKeyMismatch(t *testing.T) {
	srv := startFakeSSH(t)
	host, port := srv.hostPort()
	p := newTestProvider()
	cfg := providerCfg(host, port, "SHA256:bogusbogusbogusbogusbogusbogusbogusbogusbo")
	err := p.Validate(context.Background(), cfg, providerCreds())
	if !errors.Is(err, provider.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials on host key mismatch, got %v", err)
	}
}

func TestValidateAcceptAnyHostKey(t *testing.T) {
	srv := startFakeSSH(t)
	host, port := srv.hostPort()
	cfg := provider.Config{
		"host":                host,
		"port":                strconv.Itoa(port),
		"path":                "ninerlog-test/",
		"accept_any_host_key": true,
	}
	p := newTestProvider()
	if err := p.Validate(context.Background(), cfg, providerCreds()); err != nil {
		t.Fatalf("Validate with accept_any_host_key=true: %v", err)
	}
}

func TestUploadListDelete(t *testing.T) {
	srv := startFakeSSH(t)
	host, port := srv.hostPort()
	p := newTestProvider()
	ctx := context.Background()
	cfg := providerCfg(host, port, srv.hostKeyFP)
	creds := providerCreds()

	body := []byte("payload-data")
	res, err := p.Upload(ctx, cfg, creds, provider.UploadInput{
		Filename:    "backup-1.json.gz",
		ContentType: "application/gzip",
		Size:        int64(len(body)),
		Reader:      bytes.NewReader(body),
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if !strings.HasSuffix(res.RemotePath, "/backup-1.json.gz") {
		t.Errorf("RemotePath: %s", res.RemotePath)
	}

	// Second file.
	if _, err := p.Upload(ctx, cfg, creds, provider.UploadInput{
		Filename:    "backup-2.json.gz",
		ContentType: "application/gzip",
		Size:        4,
		Reader:      bytes.NewReader([]byte("body")),
	}); err != nil {
		t.Fatalf("Upload 2: %v", err)
	}

	objs, err := p.List(ctx, cfg, creds)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	names := map[string]bool{}
	for _, o := range objs {
		names[o.Path] = true
	}
	gotKey := ""
	for k := range names {
		if strings.HasSuffix(k, "/backup-1.json.gz") {
			gotKey = k
		}
	}
	if gotKey == "" {
		t.Errorf("expected backup-1.json.gz in list; got %+v", names)
	}

	// Delete.
	if err := p.Delete(ctx, cfg, creds, res.RemotePath); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	objs2, _ := p.List(ctx, cfg, creds)
	for _, o := range objs2 {
		if o.Path == res.RemotePath {
			t.Errorf("file not deleted: %s", o.Path)
		}
	}

	// Idempotent: deleting again is fine.
	if err := p.Delete(ctx, cfg, creds, res.RemotePath); err != nil {
		t.Errorf("expected nil on repeat delete, got %v", err)
	}
}

func TestListMissingDirReturnsEmpty(t *testing.T) {
	srv := startFakeSSH(t)
	host, port := srv.hostPort()
	cfg := providerCfg(host, port, srv.hostKeyFP)
	cfg["path"] = "does/not/exist-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "/"
	p := newTestProvider()
	objs, err := p.List(context.Background(), cfg, providerCreds())
	if err != nil {
		t.Fatalf("expected nil error on missing dir, got %v", err)
	}
	if len(objs) != 0 {
		t.Errorf("expected empty result, got %+v", objs)
	}
}

func TestParseConfigRequiresHost(t *testing.T) {
	_, _, _, err := parseConfig(provider.Config{}, providerCreds())
	if !errors.Is(err, provider.ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestParseConfigRequiresFingerprintUnlessAcceptAny(t *testing.T) {
	cfg := provider.Config{"host": "example.com"}
	_, _, _, err := parseConfig(cfg, providerCreds())
	if !errors.Is(err, provider.ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for missing fingerprint, got %v", err)
	}

	cfg["accept_any_host_key"] = true
	if _, _, _, err := parseConfig(cfg, providerCreds()); err != nil {
		t.Errorf("expected success with accept_any_host_key, got %v", err)
	}
}

func TestParseConfigRejectsBadFingerprintFormat(t *testing.T) {
	cfg := provider.Config{
		"host":                 "example.com",
		"host_key_fingerprint": "MD5:not-sha256",
	}
	_, _, _, err := parseConfig(cfg, providerCreds())
	if !errors.Is(err, provider.ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestParseConfigRejectsInvalidPort(t *testing.T) {
	cfg := provider.Config{
		"host":                 "example.com",
		"port":                 "not-a-number",
		"host_key_fingerprint": "SHA256:foo",
	}
	_, _, _, err := parseConfig(cfg, providerCreds())
	if !errors.Is(err, provider.ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}

	cfg["port"] = "0"
	if _, _, _, err := parseConfig(cfg, providerCreds()); !errors.Is(err, provider.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for port=0, got %v", err)
	}

	cfg["port"] = "65536"
	if _, _, _, err := parseConfig(cfg, providerCreds()); !errors.Is(err, provider.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for port=65536, got %v", err)
	}
}

func TestParseConfigDefaultsPathAndPort(t *testing.T) {
	cfg := provider.Config{
		"host":                 "example.com",
		"host_key_fingerprint": "SHA256:foo",
	}
	pc, _, _, err := parseConfig(cfg, providerCreds())
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if pc.Port != 22 {
		t.Errorf("default port: %d", pc.Port)
	}
	if pc.Path != "ninerlog-backups" {
		t.Errorf("default path: %s", pc.Path)
	}
}

func TestParseConfigRequiresCredentials(t *testing.T) {
	cfg := provider.Config{
		"host":                 "example.com",
		"host_key_fingerprint": "SHA256:foo",
	}
	_, _, _, err := parseConfig(cfg, provider.Credentials{})
	if !errors.Is(err, provider.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
	_, _, _, err = parseConfig(cfg, provider.Credentials{"username": "alice"})
	if !errors.Is(err, provider.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials missing password, got %v", err)
	}
}

// TestDialerTimeoutHonoured points the provider at an address that nothing
// listens on and asserts we fail quickly via the dialer.
func TestDialerTimeoutHonoured(t *testing.T) {
	// Reserve a port by listening then immediately closing — the port is now
	// almost certainly unbound. We use 127.0.0.1:1 instead to ensure refusal.
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		// Slow dialer.
		select {
		case <-time.After(2 * time.Second):
			return nil, io.EOF
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	p := NewWithDialer(dial, 100*time.Millisecond)
	cfg := provider.Config{
		"host":                 "127.0.0.1",
		"port":                 "1",
		"host_key_fingerprint": "SHA256:foo",
	}
	start := time.Now()
	err := p.Validate(context.Background(), cfg, providerCreds())
	if err == nil {
		t.Fatalf("expected error")
	}
	if time.Since(start) > time.Second {
		t.Errorf("dial timeout not honoured, elapsed=%s", time.Since(start))
	}
}
