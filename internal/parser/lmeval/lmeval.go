// Package lmeval parses EleutherAI lm-evaluation-harness result files.
package lmeval

import (
	"strings"

	"github.com/kubeshop/testkube-ai-metrics-parser/internal/jsonutil"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/lineprotocol"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/naming"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/parser"
)

const measurement = "lm_eval"

type lmEvalParser struct{}

func (lmEvalParser) Name() string { return "lm-eval" }

func (lmEvalParser) DefaultInputs() []string { return []string{"results/**/*.json", "results/*.json"} }

// Parse reads the latest results file and walks results.<task>, emitting each
// numeric metric. lm-eval encodes the filter as a suffix on the metric key
// ("exact_match,strict-match"), which we split into a `filter` tag so multiple
// filters share one metric key and stay segmentable. Stderr entries and the
// "alias" label are skipped.
func (p lmEvalParser) Parse(in parser.Input) ([]lineprotocol.Record, error) {
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
	results, ok := root["results"].(map[string]any)
	if !ok {
		return nil, nil
	}

	// Key records by task+filter so all metrics for one identity merge.
	recs := map[string]*lineprotocol.Record{}
	for task, raw := range results {
		metrics, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for key, val := range metrics {
			if key == "alias" || strings.Contains(key, "stderr") {
				continue
			}
			f, ok := jsonutil.Float(val)
			if !ok {
				continue
			}

			metricName, filter := splitMetric(key)
			structural := map[string]string{"task": task}
			if filter != "" {
				structural["filter"] = filter
			}
			tags := parser.MergeTags(in.Tags, structural)
			id := task + "\x00" + filter
			rec, ok := recs[id]
			if !ok {
				rec = &lineprotocol.Record{Measurement: measurement, Tags: tags, Fields: map[string]lineprotocol.Value{}}
				recs[id] = rec
			}
			rec.Fields[naming.Normalize(metricName)] = lineprotocol.Float(f)
		}
	}

	out := make([]lineprotocol.Record, 0, len(recs))
	for _, rec := range recs {
		if len(rec.Fields) > 0 {
			out = append(out, *rec)
		}
	}
	return out, nil
}

// splitMetric separates "exact_match,strict-match" into ("exact_match",
// "strict_match"). Keys without a comma have no filter.
func splitMetric(key string) (metric, filter string) {
	if i := strings.Index(key, ","); i >= 0 {
		return key[:i], naming.Normalize(key[i+1:])
	}
	return key, ""
}

func init() { parser.Register(lmEvalParser{}) }
