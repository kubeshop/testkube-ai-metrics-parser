package all_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kubeshop/testkube-ai-metrics-parser/internal/lineprotocol"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/parser"
	_ "github.com/kubeshop/testkube-ai-metrics-parser/internal/parser/all"
)

// Frameworks that write one file per test case (DeepEval under pytest, as in
// the rag-quality-gate workflow) must report the whole suite, not whichever
// file happens to sort last.
func TestParseAggregatesEveryMatchedFile(t *testing.T) {
	records := parseOrFail(t, "deepeval", parser.Input{Files: []string{"testdata/deepeval-multi/*.json"}})

	for _, metric := range []string{"faithfulness", "answer_relevancy"} {
		rec := recordFor(t, records, metric)
		if got, want := field(t, rec, "tests"), 3.0; got != want {
			t.Errorf("metric %s: tests = %v, want %v (one per input file)", metric, got, want)
		}
	}

	rollup := recordFor(t, records, "")
	if got, want := field(t, rollup, "passed"), 4.0; got != want {
		t.Errorf("rollup passed = %v, want %v", got, want)
	}
	if got, want := field(t, rollup, "failed"), 2.0; got != want {
		t.Errorf("rollup failed = %v, want %v", got, want)
	}
}

// --latest is the opt-out for results directories that accumulate output from
// earlier runs: only the newest file is reported.
func TestParseLatestUsesNewestFileOnly(t *testing.T) {
	records := parseOrFail(t, "deepeval", parser.Input{
		Files:  []string{"testdata/deepeval-multi/*.json"},
		Latest: true,
	})

	rec := recordFor(t, records, "faithfulness")
	if got, want := field(t, rec, "tests"), 1.0; got != want {
		t.Errorf("tests = %v, want %v", got, want)
	}
	// test_run_20260731_130032.json sorts last and scores 1.0.
	if got, want := field(t, rec, "mean_score"), 1.0; got != want {
		t.Errorf("mean_score = %v, want %v (newest file's score)", got, want)
	}
}

// A file reporting a pre-aggregated score also reports how many examples are
// behind it, and that count has to weigh the average: two shards of one task
// scoring 0.8 over 100 examples and 0.4 over 300 is 0.5 overall, not the 0.6 an
// unweighted per-file average would give.
func TestParseWeightsScoresByExampleCount(t *testing.T) {
	records := parseOrFail(t, "lm-eval", parser.Input{Files: []string{"testdata/lmeval-shards/*.json"}})

	if len(records) != 1 {
		t.Fatalf("got %d records, want 1 (both shards are the same task+filter)", len(records))
	}
	if got, want := field(t, records[0], "exact_match"), 0.5; got != want {
		t.Errorf("exact_match = %v, want %v", got, want)
	}
}

// Without a reported example count there is nothing to weigh by, so files
// contribute equally rather than being dropped.
func TestParseWeightsEquallyWithoutExampleCounts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.json"), `{"results": {"gsm8k": {"acc": 0.8}}}`)
	writeFile(t, filepath.Join(dir, "b.json"), `{"results": {"gsm8k": {"acc": 0.4}}}`)

	records := parseOrFail(t, "lm-eval", parser.Input{Files: []string{filepath.Join(dir, "*.json")}})
	if got, want := field(t, records[0], "acc"), 0.6; got != want {
		t.Errorf("acc = %v, want %v", got, want)
	}
}

// A truncated file (an aborted or timed-out run) must not discard the metrics
// of its siblings; it is skipped with a warning instead.
func TestParseSkipsUnparseableFile(t *testing.T) {
	dir := t.TempDir()
	copyFile(t, "testdata/deepeval-multi/test_run_20260731_124833.json", filepath.Join(dir, "test_run_1.json"))
	copyFile(t, "testdata/deepeval-multi/test_run_20260731_124838.json", filepath.Join(dir, "test_run_2.json"))
	writeFile(t, filepath.Join(dir, "test_run_3.json"), `{"testCases": [`)

	var warnings []string
	records := parseOrFail(t, "deepeval", parser.Input{
		Files: []string{filepath.Join(dir, "*.json")},
		Warn:  func(msg string) { warnings = append(warnings, msg) },
	})

	if got, want := field(t, recordFor(t, records, "faithfulness"), "tests"), 2.0; got != want {
		t.Errorf("tests = %v, want %v (the two readable files)", got, want)
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
}

// Skipping bad files must not hide a wholly unreadable input set.
func TestParseFailsWhenNoFileIsReadable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "test_run_1.json"), `not json`)

	p, ok := parser.Get("deepeval")
	if !ok {
		t.Fatal("deepeval not registered")
	}
	if _, err := p.Parse(parser.Input{Files: []string{filepath.Join(dir, "*.json")}}); err == nil {
		t.Error("Parse succeeded, want an error when every input file is unreadable")
	}
}

// An input glob that matches nothing is not an error: the CLI reports "no
// metrics found" and honors --fail-on-empty.
func TestParseNoMatchesIsNotAnError(t *testing.T) {
	for _, name := range parser.Names() {
		p, _ := parser.Get(name)
		records, err := p.Parse(parser.Input{Files: []string{filepath.Join(t.TempDir(), "*.json")}})
		if err != nil {
			t.Errorf("%s: Parse: %v", name, err)
		}
		if len(records) != 0 {
			t.Errorf("%s: got %d records, want 0", name, len(records))
		}
	}
}

func parseOrFail(t *testing.T, framework string, in parser.Input) []lineprotocol.Record {
	t.Helper()
	p, ok := parser.Get(framework)
	if !ok {
		t.Fatalf("framework %q not registered", framework)
	}
	records, err := p.Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("Parse returned no records")
	}
	return records
}

// recordFor returns the record tagged with the given metric name; pass "" for
// the untagged rollup line.
func recordFor(t *testing.T, records []lineprotocol.Record, metric string) lineprotocol.Record {
	t.Helper()
	for _, rec := range records {
		if rec.Tags["metric"] == metric {
			return rec
		}
	}
	t.Fatalf("no record for metric %q", metric)
	return lineprotocol.Record{}
}

func field(t *testing.T, rec lineprotocol.Record, name string) float64 {
	t.Helper()
	v, ok := rec.Fields[name]
	if !ok {
		t.Fatalf("record %v has no field %q", rec.Tags, name)
	}
	return v.AsFloat()
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("reading %s: %v", src, err)
	}
	writeFile(t, dst, string(data))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
