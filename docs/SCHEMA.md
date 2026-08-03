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

- Counts (`tests`, `passed`, `failed`, `errors`, `total_tokens`, `duration_ms`,
  `cost`) are summed.
- Scores are averaged over all the observations behind them, weighted by the
  number of observations rather than by file.
- `pass_rate` is recomputed from the summed counts.
- Structural tags still separate series: two lm-eval tasks stay two records.
- A value observed in exactly one file is emitted as-is (see the tagging policy
  above); only a genuine average is rounded.

Unparseable files are skipped with a warning, leaving the rest of the run
intact; the command fails only when no file could be parsed. `--latest`
restricts parsing to the newest matched file, for results directories that
retain output from previous runs.

## inspect-ai

- Measurement: `inspect_eval`
- Default input: `logs/*.json`
- Fields: one per scorer metric found in the logs (for example `accuracy`,
  `stderr`), averaged across logs that report the same scorer
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
  across files; repeated identities are averaged.
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
