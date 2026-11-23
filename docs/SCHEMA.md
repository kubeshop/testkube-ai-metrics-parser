# Metric schema (schema=1)

This is the canonical contract for the line protocol `testkube-ai-metrics-parser` emits. Keeping
it stable means dashboards built on these metrics stay comparable across runs,
frameworks, and users. Changes to measurement names, field names, or structural
tags are breaking and bump the schema version.

Testkube derives each metric key as `{measurement}_{field}`, lowercased, with
non-alphanumeric characters replaced by underscores. Tags become identity fields
you can segment and filter by in Insights. Only numeric fields are ingested
(integers carry an `i` suffix in the output; floats are bare).

## Tagging policy

- All records carry the explicit `--tag key=value` tags you pass (none are added
  automatically; hyperparameters are never auto-tagged).
- Some parsers add **structural** tags that are part of the metric's identity
  (for example `metric`, `task`, `filter`). These are documented per framework
  below. Structural tags win over a user tag with the same key.
- Computed values (means, rates) are rounded to 6 decimal places; values read
  directly from framework output are emitted as-is.

## Multiple input files

One evaluation run often spans several output files. Every file matched by the
effective input globs contributes to a single run-wide result, so the metric
series does not depend on how the framework chose to split its output:

- Counts (`tests`, `trials`, `passed`, `failed`, `errors`, token counts,
  `duration_ms`, `cost`, `cost_usd`) are summed.
- Scores are averaged over the examples behind them, never per file. Where a
  parser reads raw per-example data (DeepEval metric results, Ragas samples)
  this is inherent. Where a file reports a score already averaged over its own
  examples (lm-eval, inspect-ai) the file's example count weights it, so two
  shards of one task scoring 0.8 over 100 examples and 0.4 over 300 report 0.5,
  not 0.6. If a file omits its example count, files weigh equally.
- `pass_rate` is recomputed from the summed counts.
- Structural tags still separate series: two lm-eval tasks stay two records.
- A value observed in exactly one file is emitted as-is (see the tagging policy
  above); only a genuine average is rounded.

Unparseable files are skipped with a warning, leaving the rest of the run
intact; the command fails only when no file could be parsed. `--latest`
restricts parsing to the newest matched file (for Harbor, every trial of the
newest job), for results directories that retain output from previous runs.

## inspect-ai

- Measurement: `inspect_eval`
- Default input: `logs/*.json`
- Fields: one per scorer metric found in the logs (for example `accuracy`,
  `stderr`), averaged across logs that report the same scorer and weighted by
  each log's `results.completed_samples` (falling back to `total_samples`)
- Structural tags: none

## ragas

- Measurement: `ragas_eval`
- Default input: `results/ragas_results.json`
- Fields: the mean of every numeric column across the per-sample results of all
  matched files (for example `faithfulness`, `answer_relevancy`)
- Structural tags: none

## promptfoo

- Measurement: `promptfoo_eval`
- Default input: `results.json`
- Fields: `successes`, `failures`, `errors`, `pass_rate`, `total_tokens`,
  `duration_ms`, totalled over all matched files
- Structural tags: none

## lm-eval

- Measurement: `lm_eval`
- Default input: `results/**/*.json`, `results/*.json` (`**` is a single
  directory level, not a recursive match)
- Fields: one per harness metric (for example `exact_match`); `*_stderr` entries
  and the `alias` label are skipped. One record per task+filter identity, unioned
  across files; a repeated identity is averaged, weighted by the task's
  `n-samples.<task>.effective` count (falling back to `original`).
- Structural tags: `task` (benchmark name), `filter` (the harness filter, split
  out from keys like `exact_match,strict-match`)

## deepeval

- Measurement: `deepeval`
- Default input: `test_run_*.json` (the documented export produced by setting
  `DEEPEVAL_RESULTS_FOLDER`). Running under pytest writes one file per test
  case, and all of them are aggregated into the suite's totals. The internal
  `.deepeval-cache.json` can be parsed via an explicit `--input` but is not the
  default because it is an internal, version-unstable format.
- Per-metric records: fields `mean_score`, `tests`, `passed`, `failed`,
  `errors`, `cost`, tagged with `metric=<normalized metric name>`
- Rollup record: fields `passed`, `failed`, `errors` with no `metric` tag, for a
  single per-run pass/fail series
- Structural tags: `metric` (per-metric records only)

## harbor

- Measurement: `harbor_eval`
- Default input: `jobs/*/*/result.json`, the per-trial results of every job
  under Harbor's default `jobs` directory. The job-level
  `jobs/<job>/result.json` holds no per-trial data and is skipped if a wider
  glob matches it.
- One record per agent+model+dataset identity, the grouping Harbor uses for its
  own job stats.
- Fields:
  - `trials`, `errors` (trials that recorded an exception), `passed`, `failed`,
    `pass_rate`. A trial passes when the verifier reported rewards and every
    one is at least 1; `failed` is every other trial, so `passed + failed =
    trials`. An errored trial that still earned its reward counts in both
    `errors` and `passed`.
  - `mean_reward`: the mean of the `reward` key over all trials. A trial
    without a reward (a crashed agent or verifier) counts as 0. Any other
    reward key is reported the same way as `mean_<key>`.
  - `input_tokens` (includes cached tokens), `cache_tokens`, `output_tokens`,
    `total_tokens` (input + output), `cost_usd`, summed over trials. Each is
    omitted when no trial reported it, as with agents that make no model
    calls. Multi-step trials are summed over their steps. `cost_usd` is
    rounded to 9 decimal places rather than 6.
  - `duration_ms` (whole trials, including environment build and
    verification) and `agent_duration_ms` (agent execution only), summed over
    trials. Trials run concurrently, so the sum can exceed the job's
    wall-clock time.
- Structural tags: `agent`, `model` (`<provider>/<name>`, or the bare name when
  Harbor recorded no provider; absent for agents without a model), `dataset`
  (the trial's `source`: a registry dataset such as `terminal-bench@2.0`, the
  directory name for local tasks, or `adhoc` when Harbor recorded none)
- `--latest` keeps all trials of the lexicographically last job directory,
  which is the newest when job names are Harbor's default timestamps.
