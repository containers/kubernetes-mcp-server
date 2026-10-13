# KubeVirt Windows Golden Image Creator

## EULA Notice

**IMPORTANT:** By setting the `acceptEula` parameter to `"true"` in the PipelineRun below, you agree to the Microsoft Software License Terms for the Windows operating system.

Before proceeding, you **MUST** ask the user whether they accept the Microsoft End User License Agreement (EULA).

---

## PipelineRun YAML

```yaml
${yaml(steps.pipelinerun)}```

Only after the user explicitly accepts the EULA: set `acceptEula` to `"true"` and create the PipelineRun with `resources_create_or_update`. Then follow its progress with `tekton_pipelinerun_logs`.
