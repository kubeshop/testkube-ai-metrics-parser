// Package inspect parses Inspect AI eval logs.
package inspect

import (
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/jsonutil"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/lineprotocol"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/naming"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/parser"
)

// measurement is the base metric key for Inspect AI results.
const measurement = "inspect_eval"

type inspectParser struct{}

func (inspectParser) Name() string { return "inspect-ai" }

func (inspectParser) DefaultInputs() []string { return []string{"logs/*.json"} }

// Parse reads every matched Inspect log and emits each scorer metric. Inspect
// records scores as {"<metric>": {"value": <number>, ...}} nested under
// results/scores, so we collect every object of that shape anywhere in the log.
// Inspect writes one log per eval task, so a scorer reported by several logs is
// averaged across them, weighted by how many samples each log covers; keep the
// logs of unrelated tasks in separate directories (or pass a narrower --input)
// when they should stay separate series.
func (p inspectParser) Parse(in parser.Input) ([]lineprotocol.Record, error) {
	means := map[string]*parser.Mean{}
	collect := func(data any) {
		weight := logWeight(data)
		// One log may repeat a scorer at several nesting levels; only the first
		// occurrence per log counts, so a log contributes its weight once.
		seen := map[string]bool{}
		jsonutil.WalkMaps(data, func(m map[string]any) {
			for k, v := range m {
				child, ok := v.(map[string]any)
				if !ok {
					continue
				}
				raw, ok := child["value"]
				if !ok {
					continue
				}
				f, ok := jsonutil.Float(raw)
				if !ok {
					continue
				}
				name := naming.Normalize(k)
				if name == "" || seen[name] {
					continue
				}
				seen[name] = true
				if means[name] == nil {
					means[name] = &parser.Mean{}
				}
				means[name].Add(f, weight)
			}
		})
	}

	if _, err := parser.EachJSON(in, p, collect); err != nil {
		return nil, err
	}
	if len(means) == 0 {
		return nil, nil
	}

	fields := make(map[string]lineprotocol.Value, len(means))
	for name, m := range means {
		fields[name] = m.Value()
	}
	return []lineprotocol.Record{{
		Measurement: measurement,
		Tags:        parser.MergeTags(in.Tags, nil),
		Fields:      fields,
	}}, nil
}

// logWeight returns how many samples a log covers, used to weight its scores
// against other logs. Inspect keeps the count at results.completed_samples,
// falling back to total_samples; a log without either (cancelled before scoring,
// so it has no results block) weighs equally.
func logWeight(data any) float64 {
	root, ok := data.(map[string]any)
	if !ok {
		return 0
	}
	results, ok := root["results"].(map[string]any)
	if !ok {
		return 0
	}
	for _, key := range []string{"completed_samples", "total_samples"} {
		if n, ok := jsonutil.Float(results[key]); ok && n > 0 {
			return n
		}
	}
	return 0
}

func init() { parser.Register(inspectParser{}) }
