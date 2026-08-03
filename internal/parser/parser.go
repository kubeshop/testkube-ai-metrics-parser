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
	// Latest restricts parsing to the newest matched file instead of
	// aggregating every match. Use it when the results directory accumulates
	// output from earlier runs.
	Latest bool
	// Warn, when set, receives non-fatal diagnostics such as an input file that
	// could not be parsed and was skipped.
	Warn func(msg string)
}

// warnf reports a non-fatal problem, discarding it when the caller set no Warn.
func (in Input) warnf(format string, args ...any) {
	if in.Warn != nil {
		in.Warn(fmt.Sprintf(format, args...))
	}
}

// Parser converts a framework's output into line-protocol records.
//
// Parse must fold every file matched by the effective inputs into one run-wide
// result: counters sum, scores average. Frameworks routinely spread a single
// run across many files (DeepEval writes one test_run_<ts>.json per test, an
// lm-eval sweep writes one file per task), and reporting only one of them would
// undercount the run.
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
// files. Sorting is lexicographic, which for the timestamped names frameworks
// emit (for example DeepEval's test_run_<ts>.json) is also chronological, so
// Input.Latest can keep the newest file by taking the tail.
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
	if in.Latest && len(out) > 1 {
		out = out[len(out)-1:]
	}
	return out, nil
}

// EachJSON decodes every file matched by the parser's effective inputs, in
// sorted order, and hands each decoded document to fn so the parser can fold it
// into a run-wide aggregate. It returns the number of documents decoded.
//
// A file that fails to decode is skipped with a warning rather than failing the
// whole export -- a single truncated file (an aborted or timed-out run) should
// not discard the metrics of every sibling file. An error is returned only when
// no file could be decoded at all.
func EachJSON[T any](in Input, p Parser, fn func(doc T)) (int, error) {
	files, err := ResolveFiles(in, p)
	if err != nil {
		return 0, err
	}
	var (
		decoded  int
		firstErr error
	)
	for _, f := range files {
		var doc T
		if err := LoadJSON(f, &doc); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			// The error already names the file.
			in.warnf("skipping unreadable input: %v", err)
			continue
		}
		decoded++
		fn(doc)
	}
	if decoded == 0 && firstErr != nil {
		return 0, firstErr
	}
	return decoded, nil
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

// Mean accumulates repeated observations of one numeric field, which happens
// when several input files report the same metric for the same identity.
//
// Observations are weighted, because the value a file reports is usually itself
// an average over that file's examples. Averaging those per-file averages with
// equal weight would misreport unequal shards: 100 examples scoring 0.8 and 300
// scoring 0.4 is 0.5 overall, not 0.6.
type Mean struct {
	first       float64
	weightedSum float64
	weight      float64
	count       int
}

// Add records one observation covering weight examples. Pass the number of
// examples behind the value, or 0 when the framework does not report it, which
// falls back to weighting every observation equally.
func (m *Mean) Add(value, weight float64) {
	if weight <= 0 {
		weight = 1
	}
	if m.count == 0 {
		m.first = value
	}
	m.weightedSum += value * weight
	m.weight += weight
	m.count++
}

// Count returns how many observations were recorded.
func (m *Mean) Count() int { return m.count }

// Value returns the lone observation unchanged when only one file contributed,
// so source values are neither rounded nor perturbed by the weighting
// arithmetic, and the rounded weighted mean otherwise.
func (m *Mean) Value() lineprotocol.Value {
	if m.count <= 1 {
		return lineprotocol.Float(m.first)
	}
	return lineprotocol.Rounded(m.weightedSum / m.weight)
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
