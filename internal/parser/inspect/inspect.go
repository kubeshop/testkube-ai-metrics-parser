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

// Parse reads the latest Inspect log and emits each scorer metric. Inspect
// records scores as {"<metric>": {"value": <number>, ...}} nested under
// results/scores, so we collect every object of that shape anywhere in the log.
func (p inspectParser) Parse(in parser.Input) ([]lineprotocol.Record, error) {
	files, err := parser.ResolveFiles(in, p)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}

	var data any
	if err := parser.LoadJSON(parser.Latest(files), &data); err != nil {
		return nil, err
	}

	fields := map[string]lineprotocol.Value{}
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
			if name == "" {
				continue
			}
			if _, exists := fields[name]; !exists {
				fields[name] = lineprotocol.Float(f)
			}
		}
	})

	if len(fields) == 0 {
		return nil, nil
	}
	return []lineprotocol.Record{{
		Measurement: measurement,
		Tags:        parser.MergeTags(in.Tags, nil),
		Fields:      fields,
	}}, nil
}

func init() { parser.Register(inspectParser{}) }
