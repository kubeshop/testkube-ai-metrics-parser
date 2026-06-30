// Package ragas parses Ragas evaluation results.
package ragas

import (
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/jsonutil"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/lineprotocol"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/naming"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/parser"
)

const measurement = "ragas_eval"

type ragasParser struct{}

func (ragasParser) Name() string { return "ragas" }

func (ragasParser) DefaultInputs() []string { return []string{"results/ragas_results.json"} }

// Parse reads the per-sample results array (as written by
// result.to_pandas().to_json(orient="records")) and emits the mean of every
// numeric column, so adding a new Ragas metric needs no code change here.
func (p ragasParser) Parse(in parser.Input) ([]lineprotocol.Record, error) {
	files, err := parser.ResolveFiles(in, p)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}

	var samples []map[string]any
	if err := parser.LoadJSON(parser.Latest(files), &samples); err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return nil, nil
	}

	sums := map[string]float64{}
	counts := map[string]int{}
	for _, sample := range samples {
		for k, v := range sample {
			f, ok := jsonutil.Float(v)
			if !ok {
				continue
			}
			name := naming.Normalize(k)
			if name == "" {
				continue
			}
			sums[name] += f
			counts[name]++
		}
	}

	if len(sums) == 0 {
		return nil, nil
	}
	fields := make(map[string]lineprotocol.Value, len(sums))
	for name, sum := range sums {
		fields[name] = lineprotocol.Rounded(sum / float64(counts[name]))
	}
	return []lineprotocol.Record{{
		Measurement: measurement,
		Tags:        parser.MergeTags(in.Tags, nil),
		Fields:      fields,
	}}, nil
}

func init() { parser.Register(ragasParser{}) }
