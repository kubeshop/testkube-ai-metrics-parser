# Rewired example TestWorkflows

These mirror the 5 demo-org `type=ai` workflows, each with an `Export metrics`
step that runs `testkube-ai-metrics-parser` and saves a `.lp` artifact for Testkube Insights.

They reference the locally built image `testkube-ai-metrics-parser:dev` with
`imagePullPolicy: IfNotPresent`, so a cluster that already has the image loaded
(for example via `minikube image load testkube-ai-metrics-parser:dev`) uses it
without pulling from a registry.

## Prerequisites

- Testkube running in the cluster.
- The `testkube-ai-metrics-parser:dev` image loaded into the cluster:

  ```bash
  docker build -t testkube-ai-metrics-parser:dev .
  minikube image load testkube-ai-metrics-parser:dev
  ```

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

### Why the export step uses `run:` instead of `shell:`

`imagePullPolicy: IfNotPresent` only governs the kubelet pull. At scheduling
time Testkube also *inspects* each image from its registry to learn the
entrypoint, working dir, and user. For a locally-loaded-only image
(`minikube image load`) that inspection fails with `UNAUTHORIZED` against Docker
Hub and the execution is aborted before any pod starts.

Testkube skips that inspection when the step container already provides the
metadata it would otherwise fetch — concretely when `command`, `workingDir`,
and `securityContext.runAsGroup` are all set (see `NeedsImageData` in the agent
processor). So the export steps here use `run:` with an explicit `command`,
`args`, `workingDir`, and `runAsGroup`. The CLI expands input globs itself, so
no shell is required.

If you instead push the image to a registry the cluster can pull from
(GHCR, Docker Hub, an in-cluster registry), inspection succeeds and a simpler
`shell: testkube-ai-metrics-parser ...` step works too.
