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

// harborTrial renders a minimal Harbor trial result.json. extra is spliced in
// as further top-level members, for example `"exception_info": {...}`.
func harborTrial(name, agent, rewards, extra string) string {
	if extra != "" {
		extra = ", " + extra
	}
	return `{"trial_name": "` + name + `", "source": "bench",
		"agent_info": {"name": "` + agent + `", "version": "1", "model_info": {"name": "m1", "provider": "acme"}},
		"verifier_result": ` + rewards + extra + `}`
}

func writeHarborTrial(t *testing.T, root, job, trial, content string) {
	t.Helper()
	dir := filepath.Join(root, job, trial)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	writeFile(t, filepath.Join(dir, "result.json"), content)
}

// One Harbor job is a directory of per-trial files. They fold into one record
// per agent, a crashed trial counts as a zero reward instead of vanishing from
// the mean, a trial passes only when every one of its rewards is at least 1,
// and the job-level result.json matched by a loose glob is ignored.
func TestHarborAggregatesTrialsPerAgent(t *testing.T) {
	root := t.TempDir()
	writeHarborTrial(t, root, "job", "a__1", harborTrial("a__1", "alpha", `{"rewards": {"reward": 1, "style": 1}}`,
		`"agent_result": {"n_input_tokens": 100, "n_cache_tokens": 40, "n_output_tokens": 10, "cost_usd": 0.25},
		 "started_at": "2026-11-23T10:00:00Z", "finished_at": "2026-11-23T10:00:02.5Z"`))
	writeHarborTrial(t, root, "job", "a__2", harborTrial("a__2", "alpha", `{"rewards": {"reward": 1, "style": 0.5}}`,
		// A multi-step trial reports usage per step, not at the top level.
		`"agent_result": null, "step_results": [
			{"agent_result": {"n_input_tokens": 30, "n_output_tokens": 5, "cost_usd": 0.1}},
			{"agent_result": {"n_input_tokens": 20, "n_output_tokens": 5, "cost_usd": 0.15}}]`))
	writeHarborTrial(t, root, "job", "a__3", harborTrial("a__3", "alpha", `null`,
		`"exception_info": {"exception_type": "AgentTimeoutError"}`))
	writeHarborTrial(t, root, "job", "b__1", harborTrial("b__1", "beta", `{"rewards": {"reward": 1}}`, ""))
	writeFile(t, filepath.Join(root, "job", "result.json"), `{"n_total_trials": 4, "stats": {"n_errored_trials": 1}}`)

	records := parseOrFail(t, "harbor", parser.Input{Files: []string{
		filepath.Join(root, "*", "*", "result.json"),
		filepath.Join(root, "*", "result.json"),
	}})
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2 (one per agent)", len(records))
	}

	alpha := harborRecord(t, records, "alpha")
	if got, want := alpha.Tags["model"], "acme/m1"; got != want {
		t.Errorf("model tag = %q, want %q", got, want)
	}
	if got, want := alpha.Tags["dataset"], "bench"; got != want {
		t.Errorf("dataset tag = %q, want %q", got, want)
	}
	for name, want := range map[string]float64{
		"trials": 3, "passed": 1, "failed": 2, "errors": 1,
		"pass_rate":    0.333333,
		"mean_reward":  0.666667, // (1 + 1 + 0 for the crashed trial) / 3
		"mean_style":   0.5,
		"input_tokens": 150, "cache_tokens": 40, "output_tokens": 20, "total_tokens": 170,
		"cost_usd":    0.5,
		"duration_ms": 2500,
	} {
		if got := field(t, alpha, name); got != want {
			t.Errorf("alpha %s = %v, want %v", name, got, want)
		}
	}

	// An agent that reports no usage gets no token or cost fields at all,
	// rather than misleading zeros.
	beta := harborRecord(t, records, "beta")
	for _, name := range []string{"input_tokens", "total_tokens", "cost_usd", "duration_ms"} {
		if _, ok := beta.Fields[name]; ok {
			t.Errorf("beta has field %q, want it omitted", name)
		}
	}
}

// For Harbor, --latest means the newest job, with all of its trials, not the
// single newest trial file.
func TestHarborLatestUsesNewestJob(t *testing.T) {
	root := t.TempDir()
	writeHarborTrial(t, root, "2026-11-22__10-00-00", "z__1", harborTrial("z__1", "alpha", `{"rewards": {"reward": 0}}`, ""))
	writeHarborTrial(t, root, "2026-11-23__10-00-00", "a__1", harborTrial("a__1", "alpha", `{"rewards": {"reward": 1}}`, ""))
	writeHarborTrial(t, root, "2026-11-23__10-00-00", "b__1", harborTrial("b__1", "alpha", `{"rewards": {"reward": 1}}`, ""))

	records := parseOrFail(t, "harbor", parser.Input{
		Files:  []string{filepath.Join(root, "*", "*", "result.json")},
		Latest: true,
	})
	rec := harborRecord(t, records, "alpha")
	if got, want := field(t, rec, "trials"), 2.0; got != want {
		t.Errorf("trials = %v, want %v (both trials of the newest job)", got, want)
	}
	if got, want := field(t, rec, "mean_reward"), 1.0; got != want {
		t.Errorf("mean_reward = %v, want %v", got, want)
	}
}

func harborRecord(t *testing.T, records []lineprotocol.Record, agent string) lineprotocol.Record {
	t.Helper()
	for _, rec := range records {
		if rec.Tags["agent"] == agent {
			return rec
		}
	}
	t.Fatalf("no record for agent %q", agent)
	return lineprotocol.Record{}
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
