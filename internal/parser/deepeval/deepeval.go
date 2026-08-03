// Package deepeval parses DeepEval test-run output.
//
// The default input is the documented TestRun JSON export produced by setting
// DEEPEVAL_RESULTS_FOLDER (or DisplayConfig.results_folder) -- a public,
// stable surface. The internal .deepeval-cache.json can be parsed too via an
// explicit --input, since the same shape-matching logic handles both.
package deepeval

import (
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/jsonutil"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/lineprotocol"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/naming"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/parser"
)

const measurement = "deepeval"

type deepEvalParser struct{}

func (deepEvalParser) Name() string { return "deepeval" }

func (deepEvalParser) DefaultInputs() []string {
	return []string{"test_run_*.json", "*/test_run_*.json", "results/test_run_*.json"}
}

type agg struct {
	scores         []float64
	passed, failed int64
	errors         int64
	cost           float64
}

// Parse aggregates per-metric scores over every matched test run. Running
// DeepEval under pytest writes one test_run_<ts>.json per test case, so the
// files matched by a single glob are usually the individual test cases of one
// suite and belong in one set of totals.
//
// Within a file it recursively finds every MetricData-shaped object (one
// carrying name + score plus success or threshold), which tolerates the
// camel/snake field-alias differences that have appeared across DeepEval
// versions.
func (p deepEvalParser) Parse(in parser.Input) ([]lineprotocol.Record, error) {
	metrics := map[string]*agg{}
	collect := func(data any) {
		jsonutil.WalkMaps(data, func(m map[string]any) {
			name, ok := jsonutil.String(m["name"])
			if !ok {
				return
			}
			_, hasScore := m["score"]
			_, hasSuccess := m["success"]
			_, hasThreshold := m["threshold"]
			if !hasScore || (!hasSuccess && !hasThreshold) {
				return
			}

			key := naming.Normalize(name)
			if key == "" {
				return
			}
			a := metrics[key]
			if a == nil {
				a = &agg{}
				metrics[key] = a
			}

			score, scoreOK := jsonutil.Float(m["score"])
			switch {
			case !scoreOK:
				a.errors++
			default:
				a.scores = append(a.scores, score)
				if success, _ := jsonutil.Bool(m["success"]); success {
					a.passed++
				} else {
					a.failed++
				}
			}
			if cost, ok := jsonutil.Float(firstPresent(m, "evaluationCost", "evaluation_cost")); ok {
				a.cost += cost
			}
		})
	}

	if _, err := parser.EachJSON(in, p, collect); err != nil {
		return nil, err
	}
	if len(metrics) == 0 {
		return nil, nil
	}

	var (
		out                          []lineprotocol.Record
		totPassed, totFailed, totErr int64
	)
	for name, a := range metrics {
		totPassed += a.passed
		totFailed += a.failed
		totErr += a.errors

		fields := map[string]lineprotocol.Value{
			"tests":  lineprotocol.Int(int64(len(a.scores)) + a.errors),
			"passed": lineprotocol.Int(a.passed),
			"failed": lineprotocol.Int(a.failed),
			"errors": lineprotocol.Int(a.errors),
			"cost":   lineprotocol.Rounded(a.cost),
		}
		if len(a.scores) > 0 {
			fields["mean_score"] = lineprotocol.Rounded(mean(a.scores))
		}
		out = append(out, lineprotocol.Record{
			Measurement: measurement,
			Tags:        parser.MergeTags(in.Tags, map[string]string{"metric": name}),
			Fields:      fields,
		})
	}

	// Rollup line (no metric tag) for a single per-run pass/fail series.
	out = append(out, lineprotocol.Record{
		Measurement: measurement,
		Tags:        parser.MergeTags(in.Tags, nil),
		Fields: map[string]lineprotocol.Value{
			"passed": lineprotocol.Int(totPassed),
			"failed": lineprotocol.Int(totFailed),
			"errors": lineprotocol.Int(totErr),
		},
	})
	return out, nil
}

func firstPresent(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

func mean(xs []float64) float64 {
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func init() { parser.Register(deepEvalParser{}) }
