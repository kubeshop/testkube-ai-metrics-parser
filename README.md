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
- `harbor` — [Harbor](https://github.com/harbor-framework/harbor) agent eval jobs

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

Published image tags:

| Tag | Points at |
| --- | --- |
| `latest` | the tip of `main`, rebuilt on every merge |
| `main-<sha>` | one specific commit — use it to pin or roll back |
| `vX.Y.Z`, `X` | tagged releases |

`latest` tracks `main` rather than the last release, so the examples in this
repo always exercise current code. Pin to `main-<sha>` or a release tag where
you need a build that won't move under you.

## Usage

```
testkube-ai-metrics-parser <framework> [flags]

  -i, --input strings     input file or glob (repeatable; default per framework)
  -o, --output string     output .lp file path (default "metrics.lp")
  -m, --measurement       override the measurement name
      --tag key=value     identity tag (repeatable)
      --latest            parse only the newest matched file instead of aggregating all
      --fail-on-empty     exit non-zero if no metrics are found
      --stdout            also echo the line protocol to stdout
```

Each framework has a sensible default input glob (see
[docs/SCHEMA.md](docs/SCHEMA.md)), so `-i` is optional when your files are in
the conventional location.

### Multiple result files

A single evaluation run routinely produces more than one file: DeepEval under
pytest writes one `test_run_<timestamp>.json` per test case, an lm-eval sweep
writes one file per task, and sharded runs write one file per shard. All files
matched by the effective input globs are parsed and folded into **one** set of
metrics for the run:

| Kind of field | How files combine |
| --- | --- |
| Counts (`tests`, `passed`, `failed`, `errors`, `total_tokens`, `duration_ms`, `cost`) | Summed |
| Scores (`mean_score`, Ragas columns, Inspect scorers, lm-eval metrics) | Averaged over the examples behind them, not per file |
| Rates (`pass_rate`) | Recomputed from the summed counts, so a larger shard weighs more |

So a 17-test DeepEval suite reports `tests=17` with the mean score over all 17,
not whichever file sorted last. Records that carry structural tags stay
separate: lm-eval keeps one series per `task`+`filter`, and DeepEval one per
`metric`.

Scores are weighted by example count rather than by file, which matters when
files cover different numbers of examples. DeepEval and Ragas read raw
per-example data, so this is automatic. lm-eval and Inspect report a score
already averaged over their own examples, so their example counts
(`n-samples.<task>.effective` and `results.completed_samples`) do the weighting:
two shards of one task scoring 0.8 over 100 examples and 0.4 over 300 report
0.5, not the 0.6 a per-file average would give. Files that don't report a count
weigh equally.

A file that fails to parse is skipped with a warning rather than failing the
export, so one truncated file (from an aborted or timed-out run) doesn't discard
the rest. The command only fails if no file could be parsed at all.

Pass `--latest` to parse just the newest matched file (for Harbor, all trials
of the newest job), which is what you want when the results directory is
persisted across runs and still holds output from earlier ones. Files are ordered lexicographically, which matches chronological
order for the timestamped names these frameworks emit.

## Examples per framework

Each example shows the default input location and the resulting measurement.
Pass `--stdout` to also echo the line protocol, and `--tag key=value` to attach
identity tags (e.g. `--tag model=gpt-4o`).

### inspect-ai

Reads the matched Inspect AI eval logs and emits one field per scorer metric
(e.g. `accuracy`, `stderr`) under the `inspect_eval` measurement. A scorer
reported by several logs is averaged across them, weighted by how many samples
each log covers.

```bash
inspect eval task.py --model openai/gpt-4o-mini --log-dir ./logs --log-format json
testkube-ai-metrics-parser inspect-ai -i 'logs/*.json' -o metrics/eval.lp --stdout
# -> inspect_eval accuracy=1,stderr=0
```

### ragas

Reads the per-sample results arrays and emits the mean of every numeric column
(e.g. `faithfulness`, `answer_relevancy`) under `ragas_eval`. Samples from all
matched files feed the same means, so sharding a run doesn't change the result.

```bash
# ragas_eval.py writes results/ragas_results.json
testkube-ai-metrics-parser ragas -i results/ragas_results.json -o results/metrics.lp --stdout
# -> ragas_eval answer_relevancy=1,faithfulness=1
```

### promptfoo

Summarizes `results.stats` into `successes`, `failures`, `errors`, `pass_rate`,
`total_tokens`, and `duration_ms` under `promptfoo_eval`. Counts are totalled
over all matched files and `pass_rate` is derived from those totals.

```bash
npx promptfoo@latest eval -o results.json
testkube-ai-metrics-parser promptfoo -i results.json -o results.lp --stdout
# -> promptfoo_eval duration_ms=2028i,errors=0i,failures=0i,pass_rate=1,successes=4i,total_tokens=122i
```

### lm-eval

Reads the matched lm-evaluation-harness results files and emits one field per
harness metric (e.g. `exact_match`) under `lm_eval`, tagged with `task` and
`filter`. A sweep that writes one file per task produces one series per task; if
several files report the same task and filter their values are averaged,
weighted by the task's `n-samples` count. `*_stderr` entries and the `alias`
label are skipped.

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

Running under pytest writes one `test_run_<timestamp>.json` per test case, so
the glob below normally matches many files; they are aggregated into the totals
for the suite.

```bash
export DEEPEVAL_RESULTS_FOLDER=./results
deepeval test run test_evals.py
testkube-ai-metrics-parser deepeval -i 'results/test_run_*.json' -o results/metrics.lp --tag model=gpt-4o --stdout
# aggregating 17 input files (results/test_run_20260731_124833.json ... results/test_run_20260731_130032.json)
# -> deepeval,metric=answer_relevancy,model=gpt-4o cost=0.003853,errors=0i,failed=2i,mean_score=0.882353,passed=15i,tests=17i
# -> deepeval,model=gpt-4o errors=0i,failed=4i,passed=47i
```

### harbor

Reads the per-trial `result.json` files of a [Harbor](https://github.com/harbor-framework/harbor)
job and emits one record per agent, model and dataset under `harbor_eval`:
trial counts, pass rate, mean reward, token usage, cost and durations. A trial
that crashed before earning a reward counts as 0 in `mean_reward`. Token and
cost fields are left out for agents that report none.

```bash
harbor run -p tasks -a terminus-2 -m openai/gpt-5.4-mini -o jobs
testkube-ai-metrics-parser harbor -i 'jobs/*/*/result.json' -o metrics/harbor.lp --stdout
# -> harbor_eval,agent=terminus-2,dataset=tasks,model=openai/gpt-5.4-mini agent_duration_ms=11383i,cache_tokens=0i,cost_usd=0.0049245,duration_ms=59398i,errors=0i,failed=0i,input_tokens=3398i,mean_reward=1,output_tokens=528i,pass_rate=1,passed=2i,total_tokens=3926i,trials=2i
```

Harbor keeps earlier jobs in the same `jobs` directory, each in its own
subdirectory. If that directory survives between runs, pass `--latest` to
report only the newest job.

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
