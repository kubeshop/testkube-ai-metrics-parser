# testkube-ai-metrics-parser

`testkube-ai-metrics-parser` converts the output of common AI/LLM evaluation frameworks into
[InfluxDB line protocol](https://docs.influxdata.com/influxdb/v2/reference/syntax/line-protocol/)
`.lp` files that [Testkube](https://testkube.io) ingests as
[custom metrics in Insights](https://docs.testkube.io/articles/custom-metrics-in-insights).

It is a small, dependency-free Go binary. The core just parses framework output
files, so it works in any CI or observability pipeline — Testkube is the primary
consumer, not a requirement.

## Supported frameworks

- `inspect-ai` — [Inspect AI](https://inspect.aisi.org.uk/) eval logs
- `ragas` — [Ragas](https://docs.ragas.io/) evaluation results
- `promptfoo` — [promptfoo](https://www.promptfoo.dev/) eval results
- `lm-eval` — [EleutherAI lm-evaluation-harness](https://github.com/EleutherAI/lm-evaluation-harness) results
- `deepeval` — [DeepEval](https://deepeval.com/) test runs

See [docs/SCHEMA.md](docs/SCHEMA.md) for the exact measurements, fields, and
tags each one emits.

Need another framework or an enhancement to an existing one? Please
[open an issue](https://github.com/kubeshop/testkube-ai-metrics-parser/issues/new)
on the repo to request it.

## Install

```bash
# Go
go install github.com/kubeshop/testkube-ai-metrics-parser/cmd/testkube-ai-metrics-parser@latest

# Docker
docker run --rm -v "$PWD:/data" -w /data \
  docker.io/kubeshop/testkube-ai-metrics-parser:latest ragas --stdout
```

## Usage

```
testkube-ai-metrics-parser <framework> [flags]

  -i, --input strings     input file or glob (repeatable; default per framework)
  -o, --output string     output .lp file path (default "metrics.lp")
  -m, --measurement       override the measurement name
      --tag key=value     identity tag (repeatable)
      --fail-on-empty     exit non-zero if no metrics are found
      --stdout            also echo the line protocol to stdout
```

Each framework has a sensible default input glob (see
[docs/SCHEMA.md](docs/SCHEMA.md)), so `-i` is optional when your files are in
the conventional location. When a glob matches several files, the most recent
one is used.

## Examples per framework

Each example shows the default input location and the resulting measurement.
Pass `--stdout` to also echo the line protocol, and `--tag key=value` to attach
identity tags (e.g. `--tag model=gpt-4o`).

### inspect-ai

Reads the latest Inspect AI eval log and emits one field per scorer metric
(e.g. `accuracy`, `stderr`) under the `inspect_eval` measurement.

```bash
inspect eval task.py --model openai/gpt-4o-mini --log-dir ./logs --log-format json
testkube-ai-metrics-parser inspect-ai -i 'logs/*.json' -o metrics/eval.lp --stdout
# -> inspect_eval accuracy=1,stderr=0
```

### ragas

Reads the per-sample results array and emits the mean of every numeric column
(e.g. `faithfulness`, `answer_relevancy`) under `ragas_eval`.

```bash
# ragas_eval.py writes results/ragas_results.json
testkube-ai-metrics-parser ragas -i results/ragas_results.json -o results/metrics.lp --stdout
# -> ragas_eval answer_relevancy=1,faithfulness=1
```

### promptfoo

Summarizes `results.stats` into `successes`, `failures`, `errors`, `pass_rate`,
`total_tokens`, and `duration_ms` under `promptfoo_eval`.

```bash
npx promptfoo@latest eval -o results.json
testkube-ai-metrics-parser promptfoo -i results.json -o results.lp --stdout
# -> promptfoo_eval duration_ms=2028i,errors=0i,failures=0i,pass_rate=1,successes=4i,total_tokens=122i
```

### lm-eval

Reads the latest lm-evaluation-harness results file and emits one field per
harness metric (e.g. `exact_match`) under `lm_eval`, tagged with `task` and
`filter`. `*_stderr` entries and the `alias` label are skipped.

```bash
lm_eval --model local-chat-completions \
  --model_args model=gpt-4o-mini,base_url=https://api.openai.com/v1/chat/completions \
  --tasks gsm8k --apply_chat_template --output_path ./results
testkube-ai-metrics-parser lm-eval -i 'results/**/*.json' -o results/metrics.lp --stdout
# -> lm_eval,filter=flexible_extract,task=gsm8k exact_match=1
```

### deepeval

Set `DEEPEVAL_RESULTS_FOLDER` so DeepEval writes its documented
`test_run_*.json` export, then point `testkube-ai-metrics-parser` at it. Emits per-metric
records (`mean_score`, `tests`, `passed`, `failed`, `errors`, `cost`, tagged
`metric=<name>`) plus a single rollup record, under `deepeval`.

```bash
export DEEPEVAL_RESULTS_FOLDER=./results
deepeval test run test_evals.py
testkube-ai-metrics-parser deepeval -i 'results/test_run_*.json' -o results/metrics.lp --tag model=gpt-4o --stdout
# -> deepeval,metric=answer_relevancy cost=0.004848,errors=0i,failed=0i,mean_score=1,passed=1i,tests=1i
# -> deepeval errors=0i,failed=0i,passed=1i
```

## Using it in a Testkube TestWorkflow

Run the eval on its native image, then add a `testkube-ai-metrics-parser` step that reads the
output and writes a `.lp` file covered by `artifacts.paths`. Steps share the
working-directory volume, so the metrics step sees the eval's output.

```yaml
- name: Export metrics
  condition: always
  container: { image: docker.io/kubeshop/testkube-ai-metrics-parser:latest }
  shell: testkube-ai-metrics-parser ragas -i results/ragas_results.json -o results/metrics.lp --stdout
- name: Save results
  condition: always
  artifacts:
    paths: ["results/*.json", "results/*.lp"]
```

### DeepEval (Path 1, no code changes)

Existing DeepEval users keep their `deepeval test run` command and pytest files
unchanged. Just set `DEEPEVAL_RESULTS_FOLDER` so DeepEval writes its documented
`test_run_*.json` export, then point `testkube-ai-metrics-parser deepeval` at it:

```yaml
- name: Run deepeval
  container: { image: python:3.12-slim }
  env:
    - { name: DEEPEVAL_RESULTS_FOLDER, value: ./results }
    - { name: OPENAI_API_KEY, value: '{{credential("OPENAI_API_KEY")}}' }
  shell: |
    pip install -U deepeval
    deepeval test run tests/
- name: Export metrics
  condition: always
  container: { image: docker.io/kubeshop/testkube-ai-metrics-parser:latest }
  shell: testkube-ai-metrics-parser deepeval -i 'results/test_run_*.json' -o results/metrics.lp --stdout
- name: Save results
  condition: always
  artifacts:
    paths: ["results/**/*.json", "results/*.lp"]
```

For matrix runs (one model per parallel worker), run the export **inside** the
parallel block and tag it, e.g. `--tag model={{matrix.model}}`, because each
worker has its own filesystem.

## Development

```bash
make build       # build ./bin/testkube-ai-metrics-parser
make test        # go test -race ./...
make lint        # gofmt check + go vet
make golden      # regenerate golden fixtures after intentional output changes
```

Parsers self-register via an `init()` and are aggregated in
[internal/parser/all](internal/parser/all). Adding a framework means
implementing the `Parser` interface in
[internal/parser/parser.go](internal/parser/parser.go), registering it, and
adding a `testdata` fixture plus golden file.

## License

[MIT](LICENSE)
