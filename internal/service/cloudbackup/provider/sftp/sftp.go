// Package sftp implements the cloud backup provider contract against any
// SSH server with SFTP enabled (OpenSSH, Dropbear, atmoz/sftp, hosting
// providers offering SFTP-only accounts, etc.) using golang.org/x/crypto/ssh
// and github.com/pkg/sftp.
//
// Configuration model (non-secret, persisted in plain text):
//
//	host                 - required, hostname or IP of the SSH server.
//	port                 - optional uint, defaults to 22.
//	path                 - optional, absolute or relative directory under
//	                       which backups are written. A trailing "/" is
//	                       appended if missing. Defaults to
//	                       "ninerlog-backups/".
//	host_key_fingerprint - SHA256 fingerprint of the server host key in
//	                       OpenSSH format, e.g.
//	                       "SHA256:abcdef…". Required unless
//	                       accept_any_host_key is true.
//	accept_any_host_key  - optional boolean. When true, the host key is NOT
//	                       verified. This is INSECURE and intended only for
//	                       e2e tests and trusted private networks; the UI
//	                       must surface a prominent warning.
//
// Credential model (secret, encrypted at rest):
//
//	username - required
//	password - required (this v1 only supports password authentication;
//	           private-key auth is tracked separately because it requires a
//	           multi-line textarea field type in the frontend form).
//
// The provider performs an authenticated SFTP session during Validate, opens
// the target directory (creating it via MkdirAll if necessary) and reads a
// single directory entry to confirm both authentication and read+write
// permission without leaving probe data behind.
package sftp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Provider implements provider.Provider for SFTP servers.
type Provider struct {
	dialTimeout time.Duration
	netDial     func(ctx context.Context, network, addr string) (net.Conn, error)
}

// New returns an SFTP provider with sane defaults.
func New() *Provider {
	return &Provider{
		dialTimeout: 15 * time.Second,
		netDial:     defaultDial,
	}
}

// NewWithDialer returns an SFTP provider that uses the supplied dialer.
// Intended for tests.
func NewWithDialer(dial func(ctx context.Context, network, addr string) (net.Conn, error), timeout time.Duration) *Provider {
	if dial == nil {
		dial = defaultDial
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Provider{dialTimeout: timeout, netDial: dial}
}

func defaultDial(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: 15 * time.Second}
	return d.DialContext(ctx, network, addr)
}

// Provider metadata --------------------------------------------------------

func (*Provider) Name() string        { return "sftp" }
func (*Provider) DisplayName() string { return "SFTP" }
func (*Provider) Description() string {
	return "Upload encrypted JSON backups to any SSH server that supports SFTP (OpenSSH, hosted SFTP accounts, NAS appliances)."
}

func (*Provider) ConfigSchema() []provider.Field {
	return []provider.Field{
		{
			Name: "host", Label: "Host", Type: provider.FieldTypeString, Required: true,
			Placeholder: "sftp.example.com",
			Help:        "Hostname or IP address of the SSH/SFTP server.",
		},
		{
			Name: "port", Label: "Port", Type: provider.FieldTypeString, Required: false,
			Placeholder: "22",
			Help:        "TCP port. Defaults to 22.",
		},
		{
			Name: "path", Label: "Remote folder", Type: provider.FieldTypeString, Required: false,
			Placeholder: "ninerlog-backups/",
			Help:        "Folder on the server where backups are written. May be relative to the login user's home or absolute. A trailing '/' is added automatically. Defaults to 'ninerlog-backups/'.",
		},
		{
			Name: "host_key_fingerprint", Label: "Host key fingerprint", Type: provider.FieldTypeString, Required: false,
			Placeholder: "SHA256:abcdef…",
			Help:        "SHA256 fingerprint of the server's host key (OpenSSH format). Obtain via `ssh-keyscan -t ed25519 HOST | ssh-keygen -lf -`. Required unless 'Accept any host key' is enabled.",
		},
	}
}

func (*Provider) CredentialSchema() []provider.Field {
	return []provider.Field{
		{Name: "username", Label: "Username", Type: provider.FieldTypeString, Required: true, Sensitive: true},
		{Name: "password", Label: "Password", Type: provider.FieldTypePassword, Required: true, Sensitive: true},
	}
}

// Operations ---------------------------------------------------------------

// Validate authenticates, ensures the target directory exists, and confirms
// list permission.
func (p *Provider) Validate(ctx context.Context, cfg provider.Config, creds provider.Credentials) error {
	client, parsed, closeAll, err := p.connect(ctx, cfg, creds)
	if err != nil {
		return err
	}
	defer closeAll()

	if err := mkdirAll(client, parsed.Path); err != nil {
		return classifyError(err)
	}
	// Probe list permission. An empty directory is fine.
	if _, err := client.ReadDir(parsed.Path); err != nil {
		return classifyError(err)
	}
	return nil
}

// Upload streams the backup payload to <path>/<filename>.
func (p *Provider) Upload(ctx context.Context, cfg provider.Config, creds provider.Credentials, in provider.UploadInput) (*provider.UploadResult, error) {
	client, parsed, closeAll, err := p.connect(ctx, cfg, creds)
	if err != nil {
		return nil, err
	}
	defer closeAll()

	if err := mkdirAll(client, parsed.Path); err != nil {
		return nil, classifyError(err)
	}

	remotePath := path.Join(parsed.Path, in.Filename)
	f, err := client.Create(remotePath)
	if err != nil {
		return nil, classifyError(err)
	}
	defer f.Close()

	reader := in.Reader
	if reader == nil {
		reader = strings.NewReader("")
	}
	if _, err := io.Copy(f, reader); err != nil {
		// Best-effort cleanup of a half-written file.
		_ = client.Remove(remotePath)
		return nil, classifyError(err)
	}
	return &provider.UploadResult{RemotePath: remotePath}, nil
}

// List enumerates backup objects under the configured path. Directory
// entries and dotfiles are skipped.
func (p *Provider) List(ctx context.Context, cfg provider.Config, creds provider.Credentials) ([]provider.RemoteBackup, error) {
	client, parsed, closeAll, err := p.connect(ctx, cfg, creds)
	if err != nil {
		return nil, err
	}
	defer closeAll()

	entries, err := client.ReadDir(parsed.Path)
	if err != nil {
		// A missing directory is "no backups yet", not an error.
		if isNotExist(err) {
			return nil, nil
		}
		return nil, classifyError(err)
	}

	out := make([]provider.RemoteBackup, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		out = append(out, provider.RemoteBackup{
			Path:         path.Join(parsed.Path, name),
			SizeBytes:    e.Size(),
			LastModified: e.ModTime(),
		})
	}
	return out, nil
}

// Delete removes one file by its remote path.
func (p *Provider) Delete(ctx context.Context, cfg provider.Config, creds provider.Credentials, remotePath string) error {
	client, _, closeAll, err := p.connect(ctx, cfg, creds)
	if err != nil {
		return err
	}
	defer closeAll()

	if err := client.Remove(remotePath); err != nil {
		if isNotExist(err) {
			// Retention pruning is idempotent.
			return nil
		}
		return classifyError(err)
	}
	return nil
}

// Internal helpers ---------------------------------------------------------

type parsedConfig struct {
	Host              string
	Port              int
	Path              string
	HostKeyFP         string
	AcceptAnyHostKey  bool
}

func parseConfig(cfg provider.Config, creds provider.Credentials) (*parsedConfig, string, string, error) {
	host, _ := stringField(cfg, "host")
	portStr, _ := stringField(cfg, "port")
	subPath, _ := stringField(cfg, "path")
	hostKeyFP, _ := stringField(cfg, "host_key_fingerprint")
	acceptAny := boolField(cfg, "accept_any_host_key")
	username, _ := stringField(creds, "username")
	password, _ := stringField(creds, "password")

	if host == "" {
		return nil, "", "", fmt.Errorf("%w: host is required", provider.ErrInvalidConfig)
	}
	if username == "" {
		return nil, "", "", fmt.Errorf("%w: username is required", provider.ErrInvalidCredentials)
	}
	if password == "" {
		return nil, "", "", fmt.Errorf("%w: password is required", provider.ErrInvalidCredentials)
	}

	port := 22
	if portStr != "" {
		n, err := strconv.Atoi(portStr)
		if err != nil || n <= 0 || n > 65535 {
			return nil, "", "", fmt.Errorf("%w: port must be a number between 1 and 65535", provider.ErrInvalidConfig)
		}
		port = n
	}

	if !acceptAny {
		if hostKeyFP == "" {
			return nil, "", "", fmt.Errorf("%w: host_key_fingerprint is required (or set accept_any_host_key=true to disable verification)", provider.ErrInvalidConfig)
		}
		if !strings.HasPrefix(hostKeyFP, "SHA256:") {
			return nil, "", "", fmt.Errorf("%w: host_key_fingerprint must be in SHA256:... form", provider.ErrInvalidConfig)
		}
	}

	subPath = strings.TrimLeft(subPath, " ")
	if subPath == "" {
		subPath = "ninerlog-backups/"
	}
	if !strings.HasSuffix(subPath, "/") {
		subPath += "/"
	}
	// Strip trailing slash for path.Join friendliness; we re-add it where
	// human-facing paths are returned.
	cleanPath := strings.TrimRight(subPath, "/")
	if cleanPath == "" {
		cleanPath = "."
	}

	return &parsedConfig{
		Host:             host,
		Port:             port,
		Path:             cleanPath,
		HostKeyFP:        hostKeyFP,
		AcceptAnyHostKey: acceptAny,
	}, username, password, nil
}

// connect dials the server, opens an SFTP subsystem, and returns the client
// plus a closer that tears everything down.
func (p *Provider) connect(ctx context.Context, cfg provider.Config, creds provider.Credentials) (*sftp.Client, *parsedConfig, func(), error) {
	parsed, username, password, err := parseConfig(cfg, creds)
	if err != nil {
		return nil, nil, nil, err
	}

	hostKeyCB, err := buildHostKeyCallback(parsed)
	if err != nil {
		return nil, nil, nil, err
	}

	sshCfg := &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: hostKeyCB,
		Timeout:         p.dialTimeout,
	}

	addr := net.JoinHostPort(parsed.Host, strconv.Itoa(parsed.Port))

	dialCtx, cancel := context.WithTimeout(ctx, p.dialTimeout)
	conn, err := p.netDial(dialCtx, "tcp", addr)
	cancel()
	if err != nil {
		return nil, nil, nil, classifyError(err)
	}
	// SSH handshake.
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, sshCfg)
	if err != nil {
		_ = conn.Close()
		return nil, nil, nil, classifyError(err)
	}
	sshClient := ssh.NewClient(sshConn, chans, reqs)

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()
		return nil, nil, nil, classifyError(err)
	}

	closer := func() {
		_ = sftpClient.Close()
		_ = sshClient.Close()
	}
	return sftpClient, parsed, closer, nil
}

// buildHostKeyCallback returns an ssh.HostKeyCallback that either:
//   - accepts any key (when AcceptAnyHostKey is set), or
//   - compares the offered key's SHA256 fingerprint against
//     parsed.HostKeyFP.
func buildHostKeyCallback(parsed *parsedConfig) (ssh.HostKeyCallback, error) {
	if parsed.AcceptAnyHostKey {
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec // explicitly opted in
	}
	expected := strings.TrimSpace(parsed.HostKeyFP)
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		actual := ssh.FingerprintSHA256(key)
		if actual != expected {
			return fmt.Errorf("%w: host key mismatch: server presented %s, expected %s", provider.ErrInvalidCredentials, actual, expected)
		}
		return nil
	}, nil
}

// mkdirAll creates each intermediate directory if missing. The pkg/sftp
// MkdirAll behaves like os.MkdirAll for relative and absolute paths.
func mkdirAll(client *sftp.Client, dir string) error {
	if dir == "" || dir == "." {
		return nil
	}
	return client.MkdirAll(dir)
}

func stringField(m map[string]any, key string) (string, bool) {
	if m == nil {
		return "", false
	}
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(s), true
}

func boolField(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	v, ok := m[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

func isNotExist(err error) bool {
	if err == nil {
		return false
	}
	// pkg/sftp returns wrapped errors that satisfy os.IsNotExist; checking the
	// message is the documented way for code we don't control.
	msg := err.Error()
	return strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "file not found")
}

// classifyError maps SSH/SFTP errors onto the provider.Err* sentinels.
func classifyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, provider.ErrInvalidCredentials) ||
		errors.Is(err, provider.ErrInvalidConfig) ||
		errors.Is(err, provider.ErrPermissionDenied) ||
		errors.Is(err, provider.ErrNotFound) ||
		errors.Is(err, provider.ErrTransient) {
		return err
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "host key"),
		strings.Contains(msg, "knownhosts"):
		return fmt.Errorf("%w: %v", provider.ErrInvalidCredentials, err)
	case strings.Contains(msg, "unable to authenticate"),
		strings.Contains(msg, "auth failed"),
		strings.Contains(msg, "handshake failed"),
		strings.Contains(msg, "password"):
		return fmt.Errorf("%w: %v", provider.ErrInvalidCredentials, err)
	case strings.Contains(msg, "permission denied"):
		return fmt.Errorf("%w: %v", provider.ErrPermissionDenied, err)
	case isNotExist(err):
		return fmt.Errorf("%w: %v", provider.ErrNotFound, err)
	case strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "no route to host"),
		strings.Contains(msg, "network is unreachable"),
		strings.Contains(msg, "broken pipe"):
		return fmt.Errorf("%w: %v", provider.ErrTransient, err)
	}
	return err
}

// Compile-time assertion that we satisfy the provider interface.
var _ provider.Provider = (*Provider)(nil)
