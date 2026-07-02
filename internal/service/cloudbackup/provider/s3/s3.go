// Package s3 implements the cloud backup provider contract against
// Amazon S3 and S3-compatible object stores (MinIO, Backblaze B2, Cloudflare
// R2, Wasabi, etc.) using the minio-go client.
//
// Configuration model (non-secret, persisted in plain text):
//
//	bucket   - required, target bucket name
//	region   - required, e.g. "us-east-1". For S3-compatible stores that do
//	           not enforce a region, "us-east-1" is a safe default.
//	prefix   - optional, key prefix under which all backups are written.
//	           A trailing "/" is appended if missing. Defaults to
//	           "ninerlog-backups/".
//	endpoint - optional, overrides the default AWS endpoint. Required for
//	           S3-compatible stores. Both "host:port" and "https://host:port"
//	           forms are accepted; missing scheme implies HTTPS.
//	use_ssl  - optional, boolean, only consulted when endpoint is set. When
//	           true (default), connections are over TLS.
//
// Credential model (secret, encrypted at rest):
//
//	access_key_id     - required
//	secret_access_key - required
//
// The provider performs a lightweight permissions check during Validate by
// listing one object under the prefix.
package s3

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/netguard"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Provider implements provider.Provider for S3-compatible object stores.
type Provider struct {
	// httpClient is the underlying transport used for all SDK calls. Tests
	// inject a custom client to point at httptest.Server.
	httpClient *http.Client
}

// New returns an S3 provider using a hardened default HTTP client (10s
// dial/handshake, 60s response timeout) whose connections are restricted by an
// SSRF guard so user-supplied endpoints cannot reach internal addresses.
func New() *Provider {
	return &Provider{httpClient: defaultHTTPClient()}
}

// NewWithHTTPClient returns an S3 provider using the supplied HTTP client.
// Intended for tests.
func NewWithHTTPClient(c *http.Client) *Provider {
	if c == nil {
		c = defaultHTTPClient()
	}
	return &Provider{httpClient: c}
}

func defaultHTTPClient() *http.Client {
	return netguard.FromEnv().HTTPClient(60 * time.Second)
}

// Provider metadata --------------------------------------------------------

func (*Provider) Name() string        { return "s3" }
func (*Provider) DisplayName() string { return "Amazon S3 (and compatible)" }
func (*Provider) Description() string {
	return "Upload encrypted JSON backups to an Amazon S3 bucket or any S3-compatible object store (MinIO, Backblaze B2, Cloudflare R2, Wasabi)."
}

func (*Provider) ConfigSchema() []provider.Field {
	return []provider.Field{
		{Name: "bucket", Label: "Bucket", Type: provider.FieldTypeString, Required: true, Placeholder: "my-ninerlog-backups", Help: "Name of the existing bucket where backups will be written."},
		{Name: "region", Label: "Region", Type: provider.FieldTypeRegion, Required: true, Placeholder: "us-east-1", Help: "AWS region the bucket lives in, or 'us-east-1' for most S3-compatible stores."},
		{Name: "prefix", Label: "Key prefix", Type: provider.FieldTypeString, Required: false, Placeholder: "ninerlog-backups/", Help: "Optional key prefix. A trailing '/' is added automatically. Defaults to 'ninerlog-backups/'."},
		{Name: "endpoint", Label: "Endpoint URL", Type: provider.FieldTypeURL, Required: false, Placeholder: "https://s3.example.com", Help: "Leave empty for Amazon S3. Required for S3-compatible providers."},
	}
}

func (*Provider) CredentialSchema() []provider.Field {
	return []provider.Field{
		{Name: "access_key_id", Label: "Access Key ID", Type: provider.FieldTypeString, Required: true, Sensitive: true, Placeholder: "AKIAEXAMPLE"},
		{Name: "secret_access_key", Label: "Secret Access Key", Type: provider.FieldTypePassword, Required: true, Sensitive: true},
	}
}

// Operations ---------------------------------------------------------------

// Validate authenticates and confirms the bucket is reachable and writeable.
// It performs:
//  1. A 1-key prefix list (GET) to verify credentials and list permission.
//     We probe with GET before HEAD because HEAD responses carry no body, so
//     when credentials are bad some S3 implementations (SeaweedFS, and minio-go
//     itself on HEAD) collapse the error to a generic "AccessDenied" instead
//     of the real "InvalidAccessKeyId" / "SignatureDoesNotMatch" code. A GET
//     against the bucket always returns an XML <Error> body, so the real code
//     reaches classifyError.
//  2. A "BucketExists" call (HEAD on the bucket) to distinguish missing buckets
//     from auth failures once we know the credentials work.
func (p *Provider) Validate(ctx context.Context, cfg provider.Config, creds provider.Credentials) error {
	client, parsed, err := p.newClient(cfg, creds)
	if err != nil {
		return err
	}

	// Probe list permission first — GET returns an XML body even on auth
	// failure, which is necessary for accurate error classification.
	ch := client.ListObjects(ctx, parsed.Bucket, minio.ListObjectsOptions{
		Prefix:    parsed.Prefix,
		Recursive: false,
		MaxKeys:   1,
	})
	for obj := range ch {
		if obj.Err != nil {
			return classifyError(obj.Err)
		}
		// We don't actually care about the listed object — just that we got one.
		break
	}

	exists, err := client.BucketExists(ctx, parsed.Bucket)
	if err != nil {
		return classifyError(err)
	}
	if !exists {
		return fmt.Errorf("%w: bucket %q does not exist", provider.ErrNotFound, parsed.Bucket)
	}
	return nil
}

// Upload streams the backup payload to the destination using S3's multipart
// upload (the SDK chooses automatically based on size). The remote path is
// returned for storage in the BackupRun audit record.
func (p *Provider) Upload(ctx context.Context, cfg provider.Config, creds provider.Credentials, in provider.UploadInput) (*provider.UploadResult, error) {
	client, parsed, err := p.newClient(cfg, creds)
	if err != nil {
		return nil, err
	}
	key := parsed.Prefix + in.Filename
	contentType := in.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err = client.PutObject(ctx, parsed.Bucket, key, in.Reader, in.Size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return nil, classifyError(err)
	}
	return &provider.UploadResult{RemotePath: key}, nil
}

// List enumerates backup objects under the configured prefix.
func (p *Provider) List(ctx context.Context, cfg provider.Config, creds provider.Credentials) ([]provider.RemoteBackup, error) {
	client, parsed, err := p.newClient(cfg, creds)
	if err != nil {
		return nil, err
	}
	ch := client.ListObjects(ctx, parsed.Bucket, minio.ListObjectsOptions{
		Prefix:    parsed.Prefix,
		Recursive: false,
	})
	var out []provider.RemoteBackup
	for obj := range ch {
		if obj.Err != nil {
			return nil, classifyError(obj.Err)
		}
		// Skip "directory" placeholders.
		if strings.HasSuffix(obj.Key, "/") {
			continue
		}
		out = append(out, provider.RemoteBackup{
			Path:         obj.Key,
			SizeBytes:    obj.Size,
			LastModified: obj.LastModified,
		})
	}
	return out, nil
}

// Delete removes one object by its remote path.
func (p *Provider) Delete(ctx context.Context, cfg provider.Config, creds provider.Credentials, path string) error {
	client, parsed, err := p.newClient(cfg, creds)
	if err != nil {
		return err
	}
	if err := client.RemoveObject(ctx, parsed.Bucket, path, minio.RemoveObjectOptions{}); err != nil {
		return classifyError(err)
	}
	return nil
}

// Internal helpers ---------------------------------------------------------

type parsedConfig struct {
	Bucket   string
	Prefix   string
	Region   string
	Endpoint string
	Secure   bool
}

func parseConfig(cfg provider.Config, creds provider.Credentials) (*parsedConfig, string, string, error) {
	bucket, _ := stringField(cfg, "bucket")
	region, _ := stringField(cfg, "region")
	prefix, _ := stringField(cfg, "prefix")
	endpoint, _ := stringField(cfg, "endpoint")
	accessKey, _ := stringField(creds, "access_key_id")
	secretKey, _ := stringField(creds, "secret_access_key")

	if bucket == "" {
		return nil, "", "", fmt.Errorf("%w: bucket is required", provider.ErrInvalidConfig)
	}
	if region == "" {
		region = "us-east-1"
	}
	if accessKey == "" {
		return nil, "", "", fmt.Errorf("%w: access_key_id is required", provider.ErrInvalidCredentials)
	}
	if secretKey == "" {
		return nil, "", "", fmt.Errorf("%w: secret_access_key is required", provider.ErrInvalidCredentials)
	}

	prefix = strings.TrimLeft(prefix, "/")
	if prefix == "" {
		prefix = "ninerlog-backups/"
	} else if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	pc := &parsedConfig{
		Bucket: bucket,
		Prefix: prefix,
		Region: region,
		Secure: true,
	}

	if endpoint != "" {
		host, secure, err := normalizeEndpoint(endpoint)
		if err != nil {
			return nil, "", "", fmt.Errorf("%w: endpoint: %v", provider.ErrInvalidConfig, err)
		}
		pc.Endpoint = host
		pc.Secure = secure
		// Allow explicit override of TLS via use_ssl flag if present.
		if v, ok := cfg["use_ssl"]; ok {
			if b, ok := v.(bool); ok {
				pc.Secure = b
			}
		}
	}

	return pc, accessKey, secretKey, nil
}

// newClient builds a MinIO client from the supplied config + credentials.
func (p *Provider) newClient(cfg provider.Config, creds provider.Credentials) (*minio.Client, *parsedConfig, error) {
	pc, accessKey, secretKey, err := parseConfig(cfg, creds)
	if err != nil {
		return nil, nil, err
	}
	endpoint := pc.Endpoint
	if endpoint == "" {
		// AWS default endpoint
		endpoint = "s3." + pc.Region + ".amazonaws.com"
	}
	opts := &minio.Options{
		Creds:     credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:    pc.Secure,
		Region:    pc.Region,
		Transport: p.httpClient.Transport,
	}
	client, err := minio.New(endpoint, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", provider.ErrInvalidConfig, err)
	}
	return client, pc, nil
}

// normalizeEndpoint accepts forms like "https://host:port", "host:port", or
// "host", returning ("host:port", secure).
func normalizeEndpoint(raw string) (string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, errors.New("empty endpoint")
	}
	// If it parses as a URL, use its host.
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", false, err
		}
		secure := u.Scheme == "https"
		host := u.Host
		if host == "" {
			return "", false, errors.New("endpoint missing host")
		}
		return host, secure, nil
	}
	// Bare host[:port] — assume HTTPS by default.
	return raw, true, nil
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

// classifyError maps minio-go errors onto the provider.Err* sentinels so the
// service can render appropriate HTTP status codes.
func classifyError(err error) error {
	if err == nil {
		return nil
	}
	resp := minio.ToErrorResponse(err)
	switch resp.Code {
	case "AccessDenied", "AllAccessDisabled":
		return fmt.Errorf("%w: %s", provider.ErrPermissionDenied, resp.Message)
	case "InvalidAccessKeyId", "SignatureDoesNotMatch":
		return fmt.Errorf("%w: %s", provider.ErrInvalidCredentials, resp.Message)
	case "NoSuchBucket":
		return fmt.Errorf("%w: %s", provider.ErrNotFound, resp.Message)
	}
	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: %s", provider.ErrPermissionDenied, err.Error())
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%w: %s", provider.ErrInvalidCredentials, err.Error())
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %s", provider.ErrNotFound, err.Error())
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("%w: %s", provider.ErrTransient, err.Error())
	}
	return err
}
