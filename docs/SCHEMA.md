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

## inspect-ai

- Measurement: `inspect_eval`
- Default input: `logs/*.json` (latest)
- Fields: one per scorer metric found in the log (for example `accuracy`,
  `stderr`)
- Structural tags: none

## ragas

- Measurement: `ragas_eval`
- Default input: `results/ragas_results.json`
- Fields: the mean of every numeric column in the per-sample results (for
  example `faithfulness`, `answer_relevancy`)
- Structural tags: none

## promptfoo

- Measurement: `promptfoo_eval`
- Default input: `results.json`
- Fields: `successes`, `failures`, `errors`, `pass_rate`, `total_tokens`,
  `duration_ms`
- Structural tags: none

## lm-eval

- Measurement: `lm_eval`
- Default input: `results/**/*.json` (latest)
- Fields: one per harness metric (for example `exact_match`); `*_stderr` entries
  and the `alias` label are skipped
- Structural tags: `task` (benchmark name), `filter` (the harness filter, split
  out from keys like `exact_match,strict-match`)

## deepeval

- Measurement: `deepeval`
- Default input: `test_run_*.json` (the documented export produced by setting
  `DEEPEVAL_RESULTS_FOLDER`; the latest run is used). The internal
  `.deepeval-cache.json` can be parsed via an explicit `--input` but is not the
  default because it is an internal, version-unstable format.
- Per-metric records: fields `mean_score`, `tests`, `passed`, `failed`,
  `errors`, `cost`, tagged with `metric=<normalized metric name>`
- Rollup record: fields `passed`, `failed`, `errors` with no `metric` tag, for a
  single per-run pass/fail series
- Structural tags: `metric` (per-metric records only)
