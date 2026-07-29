package cloudbackup

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider/s3"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider/sftp"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider/webdav"
)

// Config used to be copied through verbatim, so a caller could set keys the
// provider reads but never declares. Three of those disabled transport
// security: accept_any_host_key (no SSH host-key verification), allow_insecure
// (plain http://) and use_ssl=false (plain HTTP to a custom S3 endpoint).
// Because they were absent from ConfigSchema the UI could not render or warn
// about them, even though the SFTP code comments assume such a warning exists.
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
