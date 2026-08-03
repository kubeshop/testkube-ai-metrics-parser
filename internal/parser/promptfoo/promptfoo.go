// Package promptfoo parses promptfoo eval result files.
package promptfoo

import (
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/jsonutil"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/lineprotocol"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/parser"
)

const measurement = "promptfoo_eval"

type promptfooParser struct{}

func (promptfooParser) Name() string { return "promptfoo" }

func (promptfooParser) DefaultInputs() []string { return []string{"results.json"} }

// Parse summarizes results.stats from every matched results file:
// success/failure/error counts, a derived pass rate, total token usage, and run
// duration. Counts, tokens, and duration are totals over all files, and the
// pass rate is recomputed from those totals rather than averaged, so a shard
// with more cases weighs proportionally more.
func (p promptfooParser) Parse(in parser.Input) ([]lineprotocol.Record, error) {
	var (
		successes, failures, errors int64
		tokens, durationMs          int64
		haveTokens, haveDuration    bool
		runs                        int
	)
	collect := func(root map[string]any) {
		stats := nestedMap(root, "results", "stats")
		if stats == nil {
			return
		}
		runs++
		successes += intField(stats, "successes")
		failures += intField(stats, "failures")
		errors += intField(stats, "errors")
		if t := nestedMap(stats, "tokenUsage"); t != nil {
			haveTokens = true
			tokens += intField(t, "total")
		}
		if v, ok := jsonutil.Float(stats["durationMs"]); ok {
			haveDuration = true
			durationMs += int64(v)
		}
	}

	if _, err := parser.EachJSON(in, p, collect); err != nil {
		return nil, err
	}
	if runs == 0 {
		return nil, nil
	}

	fields := map[string]lineprotocol.Value{
		"successes": lineprotocol.Int(successes),
		"failures":  lineprotocol.Int(failures),
		"errors":    lineprotocol.Int(errors),
	}
	passRate := 0.0
	if total := successes + failures + errors; total > 0 {
		passRate = float64(successes) / float64(total)
	}
	fields["pass_rate"] = lineprotocol.Rounded(passRate)
	if haveTokens {
		fields["total_tokens"] = lineprotocol.Int(tokens)
	}
	if haveDuration {
		fields["duration_ms"] = lineprotocol.Int(durationMs)
	}

	return []lineprotocol.Record{{
		Measurement: measurement,
		Tags:        parser.MergeTags(in.Tags, nil),
		Fields:      fields,
	}}, nil
}

func nestedMap(m map[string]any, keys ...string) map[string]any {
	cur := m
	for _, k := range keys {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

func intField(m map[string]any, key string) int64 {
	if v, ok := jsonutil.Float(m[key]); ok {
		return int64(v)
	}
	return 0
}

func init() { parser.Register(promptfooParser{}) }
