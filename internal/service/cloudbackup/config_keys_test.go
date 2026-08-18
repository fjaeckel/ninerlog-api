package cloudbackup

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider/s3"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider/sftp"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider/webdav"
)

// Every security-relevant config key a provider reads must appear in its
// ConfigSchema: accept_any_host_key (SFTP), allow_insecure (WebDAV),
// use_ssl (S3).
func TestEverySecurityRelevantConfigKeyIsDeclared(t *testing.T) {
	cases := []struct {
		name   string
		schema []provider.Field
		key    string
	}{
		{"sftp", sftp.New().ConfigSchema(), "accept_any_host_key"},
		{"webdav", webdav.New().ConfigSchema(), "allow_insecure"},
		{"s3", s3.New().ConfigSchema(), "use_ssl"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, f := range tc.schema {
				if f.Name == tc.key {
					if f.Help == "" {
						t.Errorf("%q is declared but carries no warning text", tc.key)
					}
					return
				}
			}
			t.Errorf("%q is read by the provider but not declared in ConfigSchema", tc.key)
		})
	}
}

func TestRejectUnknownFields(t *testing.T) {
	schema := []provider.Field{{Name: "host"}, {Name: "port"}}

	if err := rejectUnknownFields(schema, map[string]any{"host": "h", "port": "22"}); err != nil {
		t.Errorf("declared keys rejected: %v", err)
	}
	if err := rejectUnknownFields(schema, map[string]any{"host": "h", "sneaky_flag": true}); err == nil {
		t.Error("expected an undeclared key to be rejected")
	}
	if err := rejectUnknownFields(schema, nil); err != nil {
		t.Errorf("nil config rejected: %v", err)
	}
}
