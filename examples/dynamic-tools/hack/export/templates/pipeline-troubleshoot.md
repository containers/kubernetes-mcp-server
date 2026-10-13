# Tekton PipelineRun Troubleshooting Guide

## PipelineRun: ${args.namespace}/${args.name}

**Collected:** ${string(now)}
**Current status hint:** ${!has(steps.pr.error) && has(steps.pr.status) && has(steps.pr.status.conditions) && size(steps.pr.status.conditions) > 0 ? steps.pr.status.conditions[size(steps.pr.status.conditions) - 1].reason : 'unknown'}

Analyze the collected data and use these exact section headings in the response:

1. PipelineRun Status - overall status, conditions, and timing
2. TaskRun Status - per-task pass, failure, or blocked state
3. Failed Step Logs - errors from failed or errored steps
4. Events - related warnings and errors
5. Troubleshooting Analysis - diagnosis based on the Pipeline definition and collected context
6. Fix Suggestions - concrete next actions for the identified failure pattern

---

## PipelineRun Status

${has(steps.pr.error) ? '*Error fetching PipelineRun: ' + steps.pr.error + '*' : '```yaml\n' + yaml(steps.pr) + '```'}

---

## Pipeline Definition

${has(steps.pr.error) ? '*Pipeline definition unavailable because the PipelineRun could not be fetched*' : (has(steps.pr.spec.pipelineSpec) ? '**Embedded PipelineSpec**\n\n```yaml\n' + yaml(steps.pr.spec.pipelineSpec) + '```' : (has(steps.pr.status) && has(steps.pr.status.pipelineSpec) ? '**Resolved PipelineSpec**\n\n```yaml\n' + yaml(steps.pr.status.pipelineSpec) + '```' : (steps.pipeline == null ? '*Pipeline definition unavailable because the PipelineRun has no named Pipeline reference or embedded PipelineSpec*' : (has(steps.pipeline.error) ? '*Pipeline definition unavailable: ' + steps.pipeline.error + '*' : '```yaml\n' + yaml(steps.pipeline) + '```'))))}

---

## TaskRun Status

${size(steps.runs) == 0 ? '*No TaskRuns found for this PipelineRun*' : '```yaml\n' + yaml(steps.runs.map(tr, {'name': tr.metadata.name, 'pipelineTask': has(tr.metadata.labels) && 'tekton.dev/pipelineTask' in tr.metadata.labels ? tr.metadata.labels['tekton.dev/pipelineTask'] : '', 'conditions': has(tr.status) && has(tr.status.conditions) ? tr.status.conditions : [], 'steps': has(tr.status) && has(tr.status.steps) ? tr.status.steps : []})) + '```'}

---

## Failed Step Logs

${size(steps.runs) == 0 ? '*No TaskRuns found, so no failed or errored step logs are available*' : (size(steps.failed) == 0 ? '*No failed or errored step logs available for this PipelineRun*' : range(size(steps.failed)).map(i, '# TaskRun: ' + steps.failed[i].taskRun + ' [step: ' + steps.failed[i].name + ']\n```\n' + (type(steps.logs[i]) == string ? steps.logs[i] : 'error retrieving logs: ' + steps.logs[i].error) + '\n```').join('\n'))}

---

## Pipeline-as-Code Repositories

${has(steps.pac.error) ? '*Pipeline-as-Code Repository resources unavailable: ' + steps.pac.error + '*' : (size(steps.pac.items) == 0 ? '*No Pipeline-as-Code Repository resources found in this namespace*' : '```yaml\n' + yaml(steps.pac.items.map(r, {'name': r.metadata.name, 'spec': has(r.spec) ? r.spec : {}})) + '```')}

---

## TektonConfig

${has(steps.tektonconfigs.error) ? '*TektonConfig resources unavailable: ' + steps.tektonconfigs.error + '*' : (size(steps.tektonconfigs.items) == 0 ? '*No TektonConfig resources found*' : '```yaml\n' + yaml(steps.tektonconfigs.items.map(c, {'name': c.metadata.name, 'spec': has(c.spec) ? c.spec : {}})) + '```')}

---

## Events

${size(steps.warnings) == 0 ? '*No related warning events found*' : '```yaml\n' + yaml(steps.warnings) + '```'}

---

## Troubleshooting Analysis

Use the collected data to identify:
1. The first failed or blocked PipelineTask
2. The failed step and its relevant log error
3. Pipeline definition, parameter, workspace, dependency, or scheduling problems
4. Warning events that confirm the failure path
5. Pipeline-as-Code or TektonConfig settings that contributed to the failure

---

## Fix Suggestions

Recommend only fixes supported by the collected evidence. Common actions include correcting missing Pipeline or Task references, invalid parameters or workspaces, image pull failures, insufficient permissions or resources, and timeout or cancellation settings.
