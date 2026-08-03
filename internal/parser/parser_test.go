package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kubeshop/testkube-ai-metrics-parser/internal/lineprotocol"
)

type fakeParser struct{ defaults []string }

func (fakeParser) Name() string                               { return "fake" }
func (f fakeParser) DefaultInputs() []string                  { return f.defaults }
func (fakeParser) Parse(Input) ([]lineprotocol.Record, error) { return nil, nil }

func TestResolveFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"run_1.json", "run_2.json", "run_3.json"} {
		write(t, filepath.Join(dir, name), "{}")
	}
	glob := filepath.Join(dir, "*.json")

	t.Run("sorted and de-duplicated across overlapping globs", func(t *testing.T) {
		got, err := ResolveFiles(Input{Files: []string{glob, glob}}, fakeParser{})
		if err != nil {
			t.Fatalf("ResolveFiles: %v", err)
		}
		want := []string{
			filepath.Join(dir, "run_1.json"),
			filepath.Join(dir, "run_2.json"),
			filepath.Join(dir, "run_3.json"),
		}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	})

	t.Run("latest keeps only the newest match", func(t *testing.T) {
		got, err := ResolveFiles(Input{Files: []string{glob}, Latest: true}, fakeParser{})
		if err != nil {
			t.Fatalf("ResolveFiles: %v", err)
		}
		want := filepath.Join(dir, "run_3.json")
		if len(got) != 1 || got[0] != want {
			t.Errorf("got %v, want [%s]", got, want)
		}
	})

	t.Run("falls back to parser defaults", func(t *testing.T) {
		got, err := ResolveFiles(Input{}, fakeParser{defaults: []string{glob}})
		if err != nil {
			t.Fatalf("ResolveFiles: %v", err)
		}
		if len(got) != 3 {
			t.Errorf("got %d files, want 3", len(got))
		}
	})
}

func TestEachJSONReportsDecodedCount(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.json"), `{"n": 1}`)
	write(t, filepath.Join(dir, "b.json"), `{"n": 2}`)
	write(t, filepath.Join(dir, "c.json"), `truncated`)

	var (
		sum      float64
		warnings int
	)
	in := Input{
		Files: []string{filepath.Join(dir, "*.json")},
		Warn:  func(string) { warnings++ },
	}
	n, err := EachJSON(in, fakeParser{}, func(doc map[string]float64) { sum += doc["n"] })
	if err != nil {
		t.Fatalf("EachJSON: %v", err)
	}
	if n != 2 {
		t.Errorf("decoded %d files, want 2", n)
	}
	if sum != 3 {
		t.Errorf("summed %v, want 3", sum)
	}
	if warnings != 1 {
		t.Errorf("got %d warnings, want 1 for the truncated file", warnings)
	}
}

// Mean must weigh each observation by the examples behind it, and must not
// round or perturb a value that came straight from a single results file.
func TestMeanValue(t *testing.T) {
	type obs struct{ value, weight float64 }
	tests := []struct {
		name         string
		observations []obs
		want         float64
	}{
		{"single observation passes through", []obs{{0.30000000000000004, 1}}, 0.30000000000000004},
		{"single weighted observation is not divided", []obs{{0.1, 3}}, 0.1},
		{"average is rounded", []obs{{0.8, 1}, {0.6, 1}}, 0.7},
		{"unequal weights shift the mean", []obs{{0.8, 100}, {0.4, 300}}, 0.5},
		{"a zero weight counts as one", []obs{{0.8, 0}, {0.4, 0}}, 0.6},
		{"no observations", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m Mean
			for _, o := range tt.observations {
				m.Add(o.value, o.weight)
			}
			if got := m.Value().AsFloat(); got != tt.want {
				t.Errorf("Value() = %v, want %v", got, tt.want)
			}
			if got := m.Count(); got != len(tt.observations) {
				t.Errorf("Count() = %d, want %d", got, len(tt.observations))
			}
		})
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
