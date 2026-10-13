# KubeVirt HyperConverged Cluster Operator Status Report

## HCO: ${steps.hco.metadata.name} (namespace: ${steps.hco.metadata.namespace})

Use this report to understand the current state of the HyperConverged Cluster Operator and all components it manages.

---

## Step 1: HCO Conditions and Versions

Check the HCO conditions:
- **Available** should be True — the operator and all components are running
- **Progressing** should be False — no reconciliation in progress
- **Degraded** should be False — no component failures
- **TaintedConfiguration** indicates manual modifications to managed CRs via jsonpatch annotations
- **Upgradeable** indicates whether the operator can be safely upgraded

```yaml
${yaml({'conditions': has(steps.hco.status) && has(steps.hco.status.conditions) ? steps.hco.status.conditions : [], 'versions': has(steps.hco.status) && has(steps.hco.status.versions) ? steps.hco.status.versions : []})}```

---

## Step 2: HCO Configuration

Review the active configuration including feature gates and virtualization settings:

```yaml
${yaml(has(steps.hco.spec) ? steps.hco.spec : {})}```

---

## Step 3: Managed Component Status

HCO manages several components. Each should have its own CR with conditions:

### KubeVirt
${has(steps.kubevirts.error) ? '*Error: ' + steps.kubevirts.error + '*' : '```yaml\n' + yaml(steps.kubevirts.items.map(c, {'name': c.metadata.name, 'conditions': has(c.status) && has(c.status.conditions) ? c.status.conditions : [], 'observedKubeVirtVersion': has(c.status) && has(c.status.observedKubeVirtVersion) ? c.status.observedKubeVirtVersion : ''})) + '```'}

### CDI
${has(steps.cdis.error) ? '*Error: ' + steps.cdis.error + '*' : '```yaml\n' + yaml(steps.cdis.items.map(c, {'name': c.metadata.name, 'conditions': has(c.status) && has(c.status.conditions) ? c.status.conditions : []})) + '```'}

### NetworkAddonsConfig
${has(steps.nacs.error) ? '*Error: ' + steps.nacs.error + '*' : '```yaml\n' + yaml(steps.nacs.items.map(c, {'name': c.metadata.name, 'conditions': has(c.status) && has(c.status.conditions) ? c.status.conditions : []})) + '```'}

---

## Step 4: Events

Review events in the HCO namespace for warnings or errors:

${has(steps.events.error) ? '*Error listing events: ' + steps.events.error + '*' : (size(steps.events.items) == 0 ? '*No events found*' : '```yaml\n' + yaml(steps.events.items.filter(e, e.type == 'Warning').map(e, {'reason': has(e.reason) ? e.reason : '', 'message': has(e.message) ? e.message : '', 'object': e.involvedObject.kind + '/' + e.involvedObject.name, 'lastTimestamp': has(e.lastTimestamp) ? e.lastTimestamp : ''})) + '```')}

---

## Analysis

Based on the data above, determine:
1. **Overall Health**: Are all HCO conditions healthy (Available=True, Degraded=False)?
2. **Component Health**: Are all managed components (KubeVirt, CDI, Network Addons) available?
3. **Configuration**: Are there any non-default feature gates or virtualization settings?
4. **Warnings**: Are there any Warning events or TaintedConfiguration conditions?
5. **Versions**: What versions are installed for each component?
