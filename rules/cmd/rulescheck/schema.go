package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// schemas holds the compiled schemas by name.
type schemas map[string]*jsonschema.Schema

const schemaBase = "https://ninerlog.com/schema/rules/"

func loadSchemas(root string) (schemas, error) {
	c := jsonschema.NewCompiler()
	names := []string{"vocabulary", "messages", "rule", "record", "case", "pack"}
	for _, n := range names {
		path := filepath.Join(root, "schema", n+".schema.json")
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		doc, err := jsonschema.UnmarshalJSON(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if err := c.AddResource(schemaBase+n+".schema.json", doc); err != nil {
			return nil, err
		}
	}
	out := schemas{}
	for _, n := range names {
		s, err := c.Compile(schemaBase + n + ".schema.json")
		if err != nil {
			return nil, err
		}
		out[n] = s
	}
	return out, nil
}

// validate checks one YAML file against the named schema; it returns one line per error.
func (s schemas) validate(name, path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return []string{err.Error()}
	}
	var doc any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}
	j, err := json.Marshal(toJSON(doc))
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(j))
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}
	if err := s[name].Validate(inst); err != nil {
		var out []string
		if ve, ok := err.(*jsonschema.ValidationError); ok {
			seen := map[string]bool{}
			for _, u := range ve.BasicOutput().Errors {
				if u.Error == nil {
					continue
				}
				line := fmt.Sprintf("%s: at %s: %s", path, orRoot(u.InstanceLocation), u.Error)
				if !seen[line] {
					seen[line] = true
					out = append(out, line)
				}
			}
			if len(out) > 0 {
				return out
			}
		}
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}
	return nil
}

func orRoot(loc string) string {
	if loc == "" {
		return "/"
	}
	return loc
}

// toJSON converts a YAML tree to JSON-compatible values (dates become strings).
func toJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := map[string]any{}
		for k, x := range t {
			m[k] = toJSON(x)
		}
		return m
	case map[any]any:
		m := map[string]any{}
		for k, x := range t {
			m[fmt.Sprint(k)] = toJSON(x)
		}
		return m
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = toJSON(x)
		}
		return out
	case time.Time:
		return t.Format(time.DateOnly)
	}
	return v
}
