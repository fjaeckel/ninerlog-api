// Package provider defines the contract every cloud backup provider plugin
// must satisfy.
//
// The split between "config" (non-secret, visible to the user) and
// "credentials" (secret, encrypted at rest, only sent to the provider during
// validation/upload) is intentional and load-bearing: the API surface exposes
// configs in plain text on GET; credentials are *only* accepted on
// create/update and are never returned.
package provider

import (
	"context"
	"errors"
	"io"
	"time"
)

// FieldType enumerates the input widget kinds presented to the frontend.
// Strings deliberately match the OpenAPI BackupFieldType schema.
type FieldType string

const (
	FieldTypeString   FieldType = "string"
	FieldTypePassword FieldType = "password"
	FieldTypeRegion   FieldType = "region"
	FieldTypeURL      FieldType = "url"
)

// Field describes one field of a provider's config or credentials schema.
type Field struct {
	Name        string
	Label       string
	Type        FieldType
	Required    bool
	Help        string
	Placeholder string
	// Sensitive marks credential-bearing fields. The service refuses to log
	// or return these in API responses.
	Sensitive bool
}

// Config holds non-secret provider-specific configuration (e.g. bucket name,
// region, prefix).
type Config map[string]any

// Credentials holds secret provider-specific values (e.g. access keys).
type Credentials map[string]any

// RemoteBackup represents one previously-uploaded backup object as listed by
// the provider. Path uniquely identifies the object within the destination so
// the service can delete it during retention pruning.
type RemoteBackup struct {
	Path         string
	SizeBytes    int64
	LastModified time.Time
}

// UploadInput is the payload passed to Provider.Upload.
type UploadInput struct {
	// Reader yields the backup contents (gzipped JSON in the current
	// implementation, though the provider treats it as opaque bytes).
	Reader io.Reader
	// Size is the exact number of bytes in Reader; required for many SDKs.
	Size int64
	// Filename is a suggestion; the provider may prepend a prefix derived from
	// Config. Example: "ninerlog-backup-2025-11-19T03-00-00Z.json.gz".
	Filename string
	// ContentType describes the payload; typically "application/gzip" or
	// "application/json".
	ContentType string
}

// UploadResult is returned from Provider.Upload.
type UploadResult struct {
	// RemotePath uniquely identifies the uploaded object within the destination.
	RemotePath string
}

// Provider is the contract for a cloud storage backend.
type Provider interface {
	// Name returns the stable identifier (e.g. "s3"). Must be unique across
	// providers and match the value persisted in backup_destinations.provider.
	Name() string

	// DisplayName returns a human-readable label (e.g. "Amazon S3").
	DisplayName() string

	// Description returns user-facing copy describing the provider.
	Description() string

	// ConfigSchema describes non-secret fields shown on the create form.
	ConfigSchema() []Field

	// CredentialSchema describes secret fields shown on the create form.
	CredentialSchema() []Field

	// Validate authenticates against the provider and verifies the supplied
	// config/credentials can perform a list-and-write operation against the
	// configured location. It MUST NOT mutate user data permanently; if
	// writing a probe is necessary, the probe must be cleaned up.
	Validate(ctx context.Context, cfg Config, creds Credentials) error

	// Upload streams a backup blob to the provider and returns the path.
	Upload(ctx context.Context, cfg Config, creds Credentials, in UploadInput) (*UploadResult, error)

	// List enumerates backups previously written by NinerLog to this
	// destination. Implementations should scope the listing to the
	// configured prefix and the well-known filename convention to avoid
	// returning unrelated objects.
	List(ctx context.Context, cfg Config, creds Credentials) ([]RemoteBackup, error)

	// Delete removes one object at path.
	Delete(ctx context.Context, cfg Config, creds Credentials, path string) error
}

// Common provider errors. Wrap with fmt.Errorf("%w: ...") for additional
// context. The service surfaces these as 4xx/5xx where appropriate.
var (
	// ErrInvalidConfig indicates a required config field was missing or
	// out of range.
	ErrInvalidConfig = errors.New("provider: invalid config")
	// ErrInvalidCredentials indicates the provider rejected the credentials.
	ErrInvalidCredentials = errors.New("provider: invalid credentials")
	// ErrPermissionDenied indicates the credentials authenticated but do not
	// have the necessary permissions (e.g. cannot write to the bucket).
	ErrPermissionDenied = errors.New("provider: permission denied")
	// ErrNotFound indicates the configured location (bucket, container, etc.)
	// does not exist.
	ErrNotFound = errors.New("provider: not found")
	// ErrTransient indicates a transient remote failure that may succeed on retry.
	ErrTransient = errors.New("provider: transient failure")
)
