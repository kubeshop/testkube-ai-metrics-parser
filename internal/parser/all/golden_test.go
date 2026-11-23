package all_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/kubeshop/testkube-ai-metrics-parser/internal/lineprotocol"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/parser"
	_ "github.com/kubeshop/testkube-ai-metrics-parser/internal/parser/all"
)

var update = flag.Bool("update", false, "update .golden files")

// TestGolden verifies each parser turns a captured framework output fixture
// into the expected line protocol. Regenerate goldens with:
//
//	go test ./internal/parser/all -run TestGolden -update
func TestGolden(t *testing.T) {
	cases := []struct {
		name      string
		framework string
		input     string
		tags      map[string]string
	}{
		{"inspect", "inspect-ai", "testdata/inspect/log.json", nil},
		{"ragas", "ragas", "testdata/ragas/ragas_results.json", nil},
		{"promptfoo", "promptfoo", "testdata/promptfoo/results.json", nil},
		{"lmeval", "lm-eval", "testdata/lmeval/results.json", nil},
		{"deepeval", "deepeval", "testdata/deepeval/test_run.json", map[string]string{"model": "gpt-4o"}},

		// Captured Harbor jobs: one whose agent reports token usage, and one
		// (the nop agent) that reports none and earns no reward.
		{"harbor", "harbor", "testdata/harbor/jobs/*/*/result.json", nil},
		{"harbor-nop", "harbor", "testdata/harbor-nop/jobs/*/*/result.json", nil},

		// Globs matching several files: every framework folds the whole set
		// into one run's metrics.
		{"inspect-multi", "inspect-ai", "testdata/inspect-multi/*.json", nil},
		{"ragas-multi", "ragas", "testdata/ragas-multi/*.json", nil},
		{"promptfoo-multi", "promptfoo", "testdata/promptfoo-multi/*.json", nil},
		{"lmeval-multi", "lm-eval", "testdata/lmeval-multi/*.json", nil},
		{"deepeval-multi", "deepeval", "testdata/deepeval-multi/*.json", map[string]string{"suite": "rag-quality-gate"}},

		// Unequal shards of one task: the score is weighted by the examples
		// behind each shard, not averaged per file.
		{"lmeval-shards", "lm-eval", "testdata/lmeval-shards/*.json", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := parser.Get(tc.framework)
			if !ok {
				t.Fatalf("framework %q not registered", tc.framework)
			}
			records, err := p.Parse(parser.Input{Files: []string{tc.input}, Tags: tc.tags})
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(records) == 0 {
				t.Fatalf("Parse returned no records for %s", tc.framework)
			}
			got := lineprotocol.Serialize(records)

			goldenPath := filepath.Join("testdata", tc.name+".golden")
			if *update {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatalf("writing golden: %v", err)
				}
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("reading golden (run with -update to create): %v", err)
			}
			if got != string(want) {
				t.Errorf("line protocol mismatch for %s\n got:\n%s\nwant:\n%s", tc.framework, got, want)
			}
		})
	}
}

// TestDefaultInputs ensures every registered parser declares at least one
// default input glob.
func TestDefaultInputs(t *testing.T) {
	for _, name := range parser.Names() {
		p, _ := parser.Get(name)
		if len(p.DefaultInputs()) == 0 {
			t.Errorf("parser %q declares no default inputs", name)
		}
	}
}
