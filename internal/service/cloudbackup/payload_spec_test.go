package cloudbackup

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// The Go types are the single definition of a backup; the BackupPayload schema
// in api-spec/openapi.yaml is what generated clients see. This test walks
// Payload and ImportJSONResult through the spec so a field added, renamed or
// made optional on one side fails the build until the other side follows.
//
// Schemas named Backup*, BackupPayload, ImportJSONResult and RestoreWarning
// belong to the backup and must match exactly: same property names, and the
// required set equal to the set of tags without omitempty. Every other schema
// reached (Aircraft, Contact, NotificationPreferences, the rule and report
// definitions...) is shared with the rest of the API: it must carry every JSON
// tag, and may require only what the Go type always writes.

type schemaNode = map[string]any

func loadSpecSchemas(t *testing.T) map[string]schemaNode {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "api-spec", "openapi.yaml"))
	require.NoError(t, err)
	var doc struct {
		Components struct {
			Schemas map[string]schemaNode `yaml:"schemas"`
		} `yaml:"components"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	require.NotEmpty(t, doc.Components.Schemas)
	return doc.Components.Schemas
}

type jsonField struct {
	name      string
	omitEmpty bool
	typ       reflect.Type
}

func jsonFields(t *testing.T, typ reflect.Type) []jsonField {
	t.Helper()
	var out []jsonField
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		require.NotEmpty(t, name, "%s.%s has no json name", typ, f.Name)
		out = append(out, jsonField{name: name, omitEmpty: strings.Contains(opts, "omitempty"), typ: f.Type})
	}
	return out
}

func deref(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	return typ
}

func resolveSchema(schemas map[string]schemaNode, node schemaNode) (string, schemaNode) {
	for {
		if ref, ok := node["$ref"].(string); ok {
			name := strings.TrimPrefix(ref, "#/components/schemas/")
			node = schemas[name]
			return name, node
		}
		if items, ok := node["items"].(schemaNode); ok {
			node = items
			continue
		}
		return "", node
	}
}

func stringSet(v any) map[string]bool {
	set := map[string]bool{}
	list, _ := v.([]any)
	for _, e := range list {
		set[e.(string)] = true
	}
	return set
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func ownedByBackup(name string) bool {
	return strings.HasPrefix(name, "Backup") || name == "ImportJSONResult" || name == "RestoreWarning"
}

func checkAgainstSchema(t *testing.T, schemas map[string]schemaNode, path, name string, typ reflect.Type, node schemaNode, seen map[string]bool) {
	t.Helper()
	if typ == reflect.TypeOf(time.Time{}) || typ.Kind() != reflect.Struct {
		return
	}
	if name != "" {
		if seen[name+"|"+typ.String()] {
			return
		}
		seen[name+"|"+typ.String()] = true
	}
	props, _ := node["properties"].(schemaNode)
	require.NotNil(t, props, "%s: schema %q has no properties for %s", path, name, typ)

	fields := jsonFields(t, typ)
	goNames := map[string]bool{}
	alwaysWritten := map[string]bool{}
	for _, f := range fields {
		goNames[f.name] = true
		if !f.omitEmpty {
			alwaysWritten[f.name] = true
		}
	}
	specNames := map[string]bool{}
	for k := range props {
		specNames[k] = true
	}
	require.Equal(t, keys(goNames), keys(specNames), "%s: JSON tags of %s and properties of schema %q differ", path, typ, name)

	required := stringSet(node["required"])
	if ownedByBackup(name) {
		require.Equal(t, keys(alwaysWritten), keys(required), "%s: required list of schema %q must equal the tags of %s without omitempty", path, name, typ)
	} else {
		for r := range required {
			require.True(t, alwaysWritten[r], "%s: schema %q requires %q but %s may omit it", path, name, r, typ)
		}
	}

	for _, f := range fields {
		ft := deref(f.typ)
		if ft.Kind() != reflect.Struct || ft == reflect.TypeOf(time.Time{}) {
			continue
		}
		child, ok := props[f.name].(schemaNode)
		require.True(t, ok, "%s.%s", path, f.name)
		childName, childNode := resolveSchema(schemas, child)
		require.NotNil(t, childNode, "%s.%s: unresolved schema", path, f.name)
		checkAgainstSchema(t, schemas, path+"."+f.name, childName, ft, childNode, seen)
	}
}

func TestBackupPayloadSchemaMatchesGoTypes(t *testing.T) {
	schemas := loadSpecSchemas(t)
	seen := map[string]bool{}

	t.Run("BackupPayload", func(t *testing.T) {
		checkAgainstSchema(t, schemas, "Payload", "BackupPayload", reflect.TypeOf(Payload{}), schemas["BackupPayload"], seen)
	})
	t.Run("ImportJSONResult", func(t *testing.T) {
		checkAgainstSchema(t, schemas, "RestoreResult", "ImportJSONResult", reflect.TypeOf(RestoreResult{}), schemas["ImportJSONResult"], seen)
	})
}

func TestBackupPayloadSchemaFormatAndVersion(t *testing.T) {
	schemas := loadSpecSchemas(t)
	props := schemas["BackupPayload"]["properties"].(schemaNode)
	format := props["format"].(schemaNode)
	require.Equal(t, []any{PayloadFormat}, format["enum"])
}
