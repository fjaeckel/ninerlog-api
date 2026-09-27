package engine

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Keys is messages/keys.yaml.
type Keys struct {
	Keys []KeyDef `yaml:"keys"`
	byID map[string]*KeyDef
}

// KeyDef is one message key.
type KeyDef struct {
	Key        string   `yaml:"key"`
	Kind       string   `yaml:"kind"`
	Params     []string `yaml:"params"`
	Origin     string   `yaml:"origin"`
	Emitted    bool     `yaml:"emitted"`
	Documented bool     `yaml:"documented"`
	Deprecated bool     `yaml:"deprecated"`
	Notes      string   `yaml:"notes"`
}

// Get returns the key definition.
func (k *Keys) Get(key string) (*KeyDef, bool) {
	d, ok := k.byID[key]
	return d, ok
}

// LoadKeys reads messages/keys.yaml.
func LoadKeys(path string) (*Keys, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var k Keys
	if err := yaml.Unmarshal(b, &k); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	k.byID = map[string]*KeyDef{}
	for i := range k.Keys {
		d := &k.Keys[i]
		if _, dup := k.byID[d.Key]; dup {
			return nil, fmt.Errorf("%s: duplicate key %s", path, d.Key)
		}
		k.byID[d.Key] = d
	}
	return &k, nil
}

// Catalogue is the loaded module: vocabulary, keys and rules.
type Catalogue struct {
	Root       string
	Vocabulary *Vocabulary
	Keys       *Keys
	Rules      []*Rule
	// Errors lists rule files that could not be loaded; those rules are skipped.
	Errors []error
	byID   map[string]*Rule
}

// Rule returns the rule with the id.
func (c *Catalogue) Rule(id string) (*Rule, bool) {
	r, ok := c.byID[id]
	return r, ok
}

// LoadCatalogue reads vocabulary.yaml, messages/keys.yaml and catalogue/**.yaml under root.
func LoadCatalogue(root string) (*Catalogue, error) {
	v, err := LoadVocabulary(filepath.Join(root, "vocabulary.yaml"))
	if err != nil {
		return nil, err
	}
	k, err := LoadKeys(filepath.Join(root, "messages", "keys.yaml"))
	if err != nil {
		return nil, err
	}
	c := &Catalogue{Root: root, Vocabulary: v, Keys: k, byID: map[string]*Rule{}}
	files, err := YAMLFiles(filepath.Join(root, "catalogue"))
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		r, err := LoadRule(f)
		if err != nil {
			c.Errors = append(c.Errors, err)
			continue
		}
		if _, dup := c.byID[r.ID]; dup {
			c.Errors = append(c.Errors, fmt.Errorf("%s: duplicate rule id %s", f, r.ID))
			continue
		}
		c.byID[r.ID] = r
		c.Rules = append(c.Rules, r)
	}
	return c, nil
}

// LoadRule reads one rule file.
func LoadRule(path string) (*Rule, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Rule
	if err := yaml.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	r.File = path
	return &r, nil
}

// YAMLFiles lists *.yaml files under dir, sorted; a missing dir yields none.
func YAMLFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".yaml") {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}
