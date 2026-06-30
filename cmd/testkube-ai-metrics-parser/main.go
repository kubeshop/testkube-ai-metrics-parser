// Command testkube-ai-metrics-parser converts AI/LLM eval framework output into
// InfluxDB line protocol (.lp) for ingestion into Testkube Insights.
//
// Usage:
//
//	testkube-ai-metrics-parser <framework> [flags]
//
// See `testkube-ai-metrics-parser --help` for the framework list and flags.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kubeshop/testkube-ai-metrics-parser/internal/lineprotocol"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/parser"
	_ "github.com/kubeshop/testkube-ai-metrics-parser/internal/parser/all"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage(stdout)
		return 0
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, version)
		return 0
	}

	name := args[0]
	p, ok := parser.Get(name)
	if !ok {
		fmt.Fprintf(stderr, "unknown framework %q\n\n", name)
		usage(stderr)
		return 2
	}

	var (
		inputs      multiFlag
		tagArgs     multiFlag
		output      string
		measurement string
		failOnEmpty bool
		toStdout    bool
	)
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Var(&inputs, "input", "input file or glob (repeatable; default per framework)")
	fs.Var(&inputs, "i", "alias for --input")
	fs.StringVar(&output, "output", "metrics.lp", "output .lp file path")
	fs.StringVar(&output, "o", "metrics.lp", "alias for --output")
	fs.StringVar(&measurement, "measurement", "", "override the measurement name")
	fs.StringVar(&measurement, "m", "", "alias for --measurement")
	fs.Var(&tagArgs, "tag", "identity tag key=value (repeatable)")
	fs.BoolVar(&failOnEmpty, "fail-on-empty", false, "exit non-zero if no metrics are found")
	fs.BoolVar(&toStdout, "stdout", false, "also echo the line protocol to stdout")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	tags, err := parseTags(tagArgs)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}

	records, err := p.Parse(parser.Input{Files: inputs, Tags: tags})
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if measurement != "" {
		for i := range records {
			records[i].Measurement = measurement
		}
	}

	if len(records) == 0 {
		fmt.Fprintf(stderr, "no metrics found for %s (looked in: %s)\n", name, strings.Join(effectiveInputs(inputs, p), ", "))
		if failOnEmpty {
			return 1
		}
		return 0
	}

	out := lineprotocol.Serialize(records)
	if err := writeOutput(output, out); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if toStdout {
		fmt.Fprint(stdout, out)
	}
	return 0
}

func writeOutput(path, content string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func effectiveInputs(inputs []string, p parser.Parser) []string {
	if len(inputs) > 0 {
		return inputs
	}
	return p.DefaultInputs()
}

func parseTags(args []string) (map[string]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	tags := make(map[string]string, len(args))
	for _, a := range args {
		k, v, ok := strings.Cut(a, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid --tag %q, expected key=value", a)
		}
		tags[k] = v
	}
	return tags, nil
}

func usage(w *os.File) {
	fmt.Fprintf(w, `testkube-ai-metrics-parser %s - convert AI eval output to InfluxDB line protocol for Testkube Insights

Usage:
  testkube-ai-metrics-parser <framework> [flags]

Frameworks:
  %s

Flags:
  -i, --input strings     input file or glob (repeatable; default per framework)
  -o, --output string     output .lp file path (default "metrics.lp")
  -m, --measurement       override the measurement name
      --tag key=value     identity tag (repeatable)
      --fail-on-empty     exit non-zero if no metrics are found
      --stdout            also echo the line protocol to stdout

Examples:
  testkube-ai-metrics-parser inspect-ai -i 'logs/*.json' -o metrics/eval.lp --stdout
  testkube-ai-metrics-parser ragas -i results/ragas_results.json -o results/metrics.lp --stdout
  testkube-ai-metrics-parser promptfoo -i results.json -o results.lp --stdout
  testkube-ai-metrics-parser lm-eval -i 'results/**/*.json' -o results/metrics.lp --stdout
  testkube-ai-metrics-parser deepeval -i 'results/test_run_*.json' --tag model=gpt-4o -o results/metrics.lp
`, version, strings.Join(parser.Names(), "\n  "))
}

// multiFlag collects repeated string flags.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }

func (m *multiFlag) Set(s string) error {
	*m = append(*m, s)
	return nil
}
