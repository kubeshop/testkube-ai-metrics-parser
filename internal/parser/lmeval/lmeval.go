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

// series collects one task+filter identity across every matched results file.
type series struct {
	tags   map[string]string
	fields map[string]*parser.Mean
}

// Parse walks results.<task> in every matched results file, emitting each
// numeric metric. A harness sweep writes one file per task, so the files are
// unioned by task+filter identity; when several files do report the same
// identity (a re-run, or the same task in two shards) their values are
// averaged. lm-eval encodes the filter as a suffix on the metric key
// ("exact_match,strict-match"), which we split into a `filter` tag so multiple
// filters share one metric key and stay segmentable. Stderr entries and the
// "alias" label are skipped.
func (p lmEvalParser) Parse(in parser.Input) ([]lineprotocol.Record, error) {
	all := map[string]*series{}
	collect := func(root map[string]any) {
		results, ok := root["results"].(map[string]any)
		if !ok {
			return
		}
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
				id := task + "\x00" + filter
				s, ok := all[id]
				if !ok {
					structural := map[string]string{"task": task}
					if filter != "" {
						structural["filter"] = filter
					}
					s = &series{
						tags:   parser.MergeTags(in.Tags, structural),
						fields: map[string]*parser.Mean{},
					}
					all[id] = s
				}
				name := naming.Normalize(metricName)
				if s.fields[name] == nil {
					s.fields[name] = &parser.Mean{}
				}
				s.fields[name].Add(f)
			}
		}
	}

	if _, err := parser.EachJSON(in, p, collect); err != nil {
		return nil, err
	}

	out := make([]lineprotocol.Record, 0, len(all))
	for _, s := range all {
		if len(s.fields) == 0 {
			continue
		}
		fields := make(map[string]lineprotocol.Value, len(s.fields))
		for name, m := range s.fields {
			fields[name] = m.Value()
		}
		out = append(out, lineprotocol.Record{Measurement: measurement, Tags: s.tags, Fields: fields})
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
