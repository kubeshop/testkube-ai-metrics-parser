# Rewired example TestWorkflows

These mirror the 5 demo-org `type=ai` workflows, each with an `Export metrics`
step that runs `testkube-ai-metrics-parser` and saves a `.lp` artifact for Testkube Insights.

The export steps pull the published image
`docker.io/kubeshop/testkube-ai-metrics-parser:latest` from Docker Hub.

## Prerequisites

- Testkube running in the cluster, able to pull from Docker Hub.
- An `OPENAI_API_KEY` credential created in the Testkube environment (not
  needed for `ai-metrics-smoke.yaml`). The workflows reference it as
  `value: '{{credential("OPENAI_API_KEY")}}'`, matching the demo-environment
  workflows.

## Apply and run

Start with the smoke test (no API key, fastest signal that the image works
in-cluster):

```bash
kubectl testkube create testworkflow -f examples/workflows/ai-metrics-smoke.yaml
kubectl testkube run testworkflow ai-metrics-smoke -f
```

Then the real ones:

```bash
for f in ragas inspect-ai promptfoo lm-eval deepeval; do
  kubectl testkube create testworkflow -f "examples/workflows/$f.yaml"
done
kubectl testkube run testworkflow ragas-getting-started -f
```

After a run, check the execution's Artifacts tab for `metrics.lp` (or
`results.lp` / `eval.lp`) and the Insights tab for the ingested series.

## Notes

- The export step uses a different image than the eval step; TestWorkflow steps
  share the working-directory volume, so it sees the eval's output files.
- For matrix workflows (one model per parallel worker), put the export step
  inside the `parallel` block and tag it, e.g. `--tag model={{matrix.model}}`,
  because each worker has its own filesystem.
- The image runs as root so it can write its `.lp` output into the shared
  workspace alongside files created by the (typically root) eval step.
