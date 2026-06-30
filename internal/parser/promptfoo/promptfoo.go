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

// Parse reads results.json and summarizes results.stats: success/failure/error
// counts, a derived pass rate, total token usage, and run duration.
func (p promptfooParser) Parse(in parser.Input) ([]lineprotocol.Record, error) {
	files, err := parser.ResolveFiles(in, p)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}

	var root map[string]any
	if err := parser.LoadJSON(parser.Latest(files), &root); err != nil {
		return nil, err
	}

	stats := nestedMap(root, "results", "stats")
	if stats == nil {
		return nil, nil
	}

	successes := intField(stats, "successes")
	failures := intField(stats, "failures")
	errors := intField(stats, "errors")
	total := successes + failures + errors

	fields := map[string]lineprotocol.Value{
		"successes": lineprotocol.Int(successes),
		"failures":  lineprotocol.Int(failures),
		"errors":    lineprotocol.Int(errors),
	}
	passRate := 0.0
	if total > 0 {
		passRate = float64(successes) / float64(total)
	}
	fields["pass_rate"] = lineprotocol.Rounded(passRate)

	if tokens := nestedMap(stats, "tokenUsage"); tokens != nil {
		fields["total_tokens"] = lineprotocol.Int(intField(tokens, "total"))
	}
	if v, ok := jsonutil.Float(stats["durationMs"]); ok {
		fields["duration_ms"] = lineprotocol.Int(int64(v))
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
