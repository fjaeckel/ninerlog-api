package main

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/fjaeckel/ninerlog-rules/engine"
)

var headerURL = regexp.MustCompile(`https?://[^\s)>\]]+`)

// sourcesResult is the outcome of the sources/ provenance check.
type sourcesResult struct {
	origins  map[string]int
	problems []string
}

// checkSources enforces that sources/ holds only verbatim texts of an allowed origin
// (vocabulary.yaml source_origins; DESIGN.md section 13): every file is Markdown, declares an
// allowed origin in its header, lives in that origin's directory, carries the origin's
// attribution, cites only the origin's hosts, and names no forbidden material in its header
// or headings.
func checkSources(root string, v *engine.Vocabulary) (sourcesResult, error) {
	res := sourcesResult{origins: map[string]int{}}
	dir := filepath.Join(root, "sources")
	markers := make([]*regexp.Regexp, 0, len(v.SourceForbidden))
	for _, m := range v.SourceForbidden {
		// A marker matches as a word, followed by optional digits (AMC1, GM2), with spaces
		// matching hyphens or underscores too (easy-access-rules).
		words := strings.Fields(m)
		for i, w := range words {
			words[i] = regexp.QuoteMeta(w)
		}
		markers = append(markers, regexp.MustCompile(`(?i)\b`+strings.Join(words, `[\s_-]+`)+`\d*\b`))
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == filepath.Join("sources", "README.md") {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			res.problems = append(res.problems, rel+": only Markdown texts are stored under sources/ (no raw downloads)")
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, p := range checkSourceFile(rel, string(b), v, markers) {
			res.problems = append(res.problems, rel+": "+p)
		}
		if origin := headerField(string(b), "Origin"); origin != "" {
			res.origins[origin]++
		}
		return nil
	})
	sort.Strings(res.problems)
	return res, err
}

// sourceHeader returns the "- Key: value" lines before the first heading or rule.
func sourceHeader(text string) []string {
	var out []string
	for i, line := range strings.Split(text, "\n") {
		if i > 0 && (strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "---")) {
			break
		}
		if strings.HasPrefix(line, "- ") {
			out = append(out, line)
		}
	}
	return out
}

func headerField(text, key string) string {
	for _, l := range sourceHeader(text) {
		if v, ok := strings.CutPrefix(l, "- "+key+": "); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func checkSourceFile(rel, text string, v *engine.Vocabulary, markers []*regexp.Regexp) []string {
	var out []string
	origin := headerField(text, "Origin")
	def, ok := v.SourceOrigins[origin]
	if !ok {
		allowed := keys(v.SourceOrigins)
		return []string{fmt.Sprintf("header has no allowed origin (\"- Origin: \" one of %v)", allowed)}
	}
	if parts := strings.Split(filepath.ToSlash(rel), "/"); len(parts) != 3 || parts[1] != def.Directory {
		out = append(out, fmt.Sprintf("origin %s belongs in sources/%s/", origin, def.Directory))
	}
	if !strings.Contains(headerField(text, "Attribution"), def.Attribution) {
		out = append(out, fmt.Sprintf("header needs an \"- Attribution:\" line containing %q", def.Attribution))
	}
	header := sourceHeader(text)
	for _, l := range header {
		for _, u := range headerURL.FindAllString(l, -1) {
			p, err := url.Parse(u)
			if err != nil || !slices.Contains(def.Hosts, p.Hostname()) {
				out = append(out, fmt.Sprintf("header cites %s, which is not a host of origin %s %v", u, origin, def.Hosts))
			}
		}
	}
	lines := slices.Clone(header)
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, "#") {
			lines = append(lines, l)
		}
	}
	for _, l := range lines {
		if strings.HasPrefix(l, "- Attribution:") {
			continue
		}
		for i, m := range markers {
			if m.MatchString(l) {
				out = append(out, fmt.Sprintf("names %q in its header or a heading; such material is never stored (%q)", v.SourceForbidden[i], abbreviate(l)))
			}
		}
	}
	return out
}
