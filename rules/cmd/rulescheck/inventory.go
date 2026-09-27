package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fjaeckel/ninerlog-rules/engine"
	"gopkg.in/yaml.v3"
)

// mapsTo accepts a rule id, a list of ids or nothing.
type mapsTo []string

func (m *mapsTo) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Tag != "!!null" && strings.TrimSpace(n.Value) != "" {
			*m = mapsTo{n.Value}
		}
		return nil
	case yaml.SequenceNode:
		var l []string
		if err := n.Decode(&l); err != nil {
			return err
		}
		*m = l
	}
	return nil
}

// checkCodeRules finds inventory/code-rules.yaml entries without a catalogue rule.
func checkCodeRules(root string, cat *engine.Catalogue) (inventoryResult, error) {
	var res inventoryResult
	var doc struct {
		Rules []struct {
			ID     string `yaml:"id"`
			Scope  string `yaml:"scope"`
			MapsTo mapsTo `yaml:"maps_to"`
		} `yaml:"rules"`
	}
	if err := readYAML(filepath.Join(root, "inventory", "code-rules.yaml"), &doc); err != nil {
		return res, err
	}
	for _, e := range doc.Rules {
		res.total++
		if e.Scope == "consumer" {
			res.excluded++
			continue
		}
		if len(e.MapsTo) == 0 {
			res.unmapped = append(res.unmapped, e.ID)
		}
		for _, id := range e.MapsTo {
			if _, ok := cat.Rule(id); !ok {
				res.unknown = append(res.unknown, fmt.Sprintf("%s -> %s", e.ID, id))
			}
		}
	}
	return res, nil
}

// checkArticles finds inventory/articles-*.yaml entries without a catalogue entry.
func checkArticles(root string, cat *engine.Catalogue) (map[string]inventoryResult, error) {
	files, err := filepath.Glob(filepath.Join(root, "inventory", "articles-*.yaml"))
	if err != nil {
		return nil, err
	}
	out := map[string]inventoryResult{}
	for _, f := range files {
		var doc struct {
			Articles []struct {
				Cite   string `yaml:"cite"`
				MapsTo mapsTo `yaml:"maps_to"`
			} `yaml:"articles"`
		}
		if err := readYAML(f, &doc); err != nil {
			return nil, err
		}
		auth := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "articles-"), ".yaml")
		var res inventoryResult
		for _, a := range doc.Articles {
			res.total++
			if len(a.MapsTo) == 0 {
				res.unmapped = append(res.unmapped, a.Cite)
			}
			for _, id := range a.MapsTo {
				if _, ok := cat.Rule(id); !ok {
					res.unknown = append(res.unknown, fmt.Sprintf("%s -> %s", a.Cite, id))
				}
			}
		}
		out[auth] = res
	}
	return out, nil
}

func readYAML(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// engineCoverage runs the engine tests and returns the combined statement coverage.
func engineCoverage(root string) (float64, error) {
	tmp, err := os.CreateTemp("", "rulescheck-cover-*.out")
	if err != nil {
		return 0, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	cmd := exec.Command("go", "test", "-count=1", "-coverpkg=./engine/...", "-coverprofile="+tmp.Name(), "./engine/...")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("go test ./engine/...: %v\n%s", err, out)
	}
	return profileCoverage(tmp.Name())
}

// profileCoverage computes covered statements over all blocks of a cover profile.
func profileCoverage(path string) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	type block struct{ stmts, count int }
	blocks := map[string]block{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "mode:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		stmts, _ := strconv.Atoi(fields[1])
		count, _ := strconv.Atoi(fields[2])
		b := blocks[fields[0]]
		b.stmts = stmts
		b.count += count
		blocks[fields[0]] = b
	}
	total, covered := 0, 0
	for _, b := range blocks {
		total += b.stmts
		if b.count > 0 {
			covered += b.stmts
		}
	}
	if total == 0 {
		return 0, fmt.Errorf("empty cover profile")
	}
	return 100 * float64(covered) / float64(total), sc.Err()
}
