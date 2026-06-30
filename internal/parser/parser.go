// Package parser defines the framework-parser contract and a registry of
// built-in parsers. A Parser reads one AI-eval framework's output files and
// produces line-protocol records; it never reads hyperparameters or otherwise
// invents identity tags beyond the structural ones defined by its schema.
package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/kubeshop/testkube-ai-metrics-parser/internal/lineprotocol"
)

// Input carries the resolved invocation parameters handed to a parser.
type Input struct {
	// Files holds raw globs/paths from --input. When empty the parser falls
	// back to its DefaultInputs.
	Files []string
	// Tags are explicit user-supplied identity tags (--tag key=value), applied
	// to every emitted record. Parsers merge their own structural tags on top.
	Tags map[string]string
}

// Parser converts a framework's output into line-protocol records.
type Parser interface {
	Name() string
	DefaultInputs() []string
	Parse(in Input) ([]lineprotocol.Record, error)
}

var registry = map[string]Parser{}

// Register adds p to the global registry. Intended to be called from parser
// package init functions.
func Register(p Parser) {
	registry[p.Name()] = p
}

// Get looks up a registered parser by name.
func Get(name string) (Parser, bool) {
	p, ok := registry[name]
	return p, ok
}

// Names returns the sorted list of registered framework names.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ResolveFiles expands the parser's effective globs (Input.Files or, when that
// is empty, the parser defaults) into a sorted, de-duplicated list of existing
// files.
func ResolveFiles(in Input, p Parser) ([]string, error) {
	globs := in.Files
	if len(globs) == 0 {
		globs = p.DefaultInputs()
	}
	seen := map[string]bool{}
	var out []string
	for _, g := range globs {
		matches, err := filepath.Glob(g)
		if err != nil {
			return nil, fmt.Errorf("invalid input pattern %q: %w", g, err)
		}
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// Latest returns the lexicographically last file, which for timestamp-prefixed
// names (for example DeepEval's test_run_<ts>.json) is the most recent run.
// Returns "" for an empty slice.
func Latest(files []string) string {
	if len(files) == 0 {
		return ""
	}
	return files[len(files)-1]
}

// LoadJSON reads and unmarshals a JSON file into v.
func LoadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	return nil
}

// MergeTags returns a new tag map combining user-supplied tags with a parser's
// structural tags. Structural tags win on conflict so the schema stays intact.
func MergeTags(user map[string]string, structural map[string]string) map[string]string {
	out := make(map[string]string, len(user)+len(structural))
	for k, v := range user {
		out[k] = v
	}
	for k, v := range structural {
		out[k] = v
	}
	return out
}
