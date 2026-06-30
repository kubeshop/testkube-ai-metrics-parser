// Package all blank-imports every built-in parser so that importing it
// registers the full set with the parser registry.
package all

import (
	_ "github.com/kubeshop/testkube-ai-metrics-parser/internal/parser/deepeval"
	_ "github.com/kubeshop/testkube-ai-metrics-parser/internal/parser/inspect"
	_ "github.com/kubeshop/testkube-ai-metrics-parser/internal/parser/lmeval"
	_ "github.com/kubeshop/testkube-ai-metrics-parser/internal/parser/promptfoo"
	_ "github.com/kubeshop/testkube-ai-metrics-parser/internal/parser/ragas"
)
