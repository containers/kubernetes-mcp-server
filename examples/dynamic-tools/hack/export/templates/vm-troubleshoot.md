# KubeVirt VirtualMachine Troubleshooting Guide

## VM: ${args.name} (namespace: ${args.namespace})

Use this guide to diagnose issues with the KubeVirt VirtualMachine. The relevant resource data has been collected below.

---

## Step 1: VirtualMachine Status

Check the VirtualMachine status:
- printableStatus (should be "Running")
- ready (should be true)
- conditions for any errors

${has(steps.vm.error) ? '*Error fetching VirtualMachine: ' + steps.vm.error + '*' : '```yaml\n' + yaml(has(steps.vm.status) ? steps.vm.status : {}) + '```'}

---

## Step 2: VirtualMachineInstance Status

Check the VirtualMachineInstance status:
- phase (should be "Running")
- conditions for "Ready" condition

${has(steps.vmi.error) ? '*Error fetching VirtualMachineInstance: ' + steps.vmi.error + '*' : '```yaml\n' + yaml(has(steps.vmi.status) ? steps.vmi.status : {}) + '```'}

---

## Step 3: VM Volumes

Review the VM volumes for any configuration issues (cloud-init bodies are redacted):

${has(steps.vm.error) ? '*VirtualMachine unavailable, no volumes to review*' : '```yaml\n' + yaml((has(steps.vm.spec) && has(steps.vm.spec.template) && has(steps.vm.spec.template.spec) && has(steps.vm.spec.template.spec.volumes) ? steps.vm.spec.template.spec.volumes : []).map(v, v.transformMapEntry(k, val, k.startsWith('cloudInit') ? {k: '<redacted>'} : {k: val}))) + '```'}

**Common volume types to check:**
- **containerDisk**: Verify the image URL is accessible and the image exists
- **cloudInitNoCloud / cloudInitConfigDrive**: Check userData and networkData for syntax errors
- **configMap**: Verify the referenced ConfigMap exists in the namespace
- **secret**: Verify the referenced Secret exists in the namespace
- **serviceAccount**: Check if the ServiceAccount exists and has proper permissions
- **emptyDisk**: Usually no issues, but check if capacity is specified correctly
- **hostDisk**: Verify the path exists on the host node and has correct permissions
- **downwardMetrics**: Usually no configuration issues
- **dataVolume**: Check that the DataVolume exists and status.phase is "Succeeded"
- **persistentVolumeClaim**: Check that the PVC exists and status.phase is "Bound"

---

## Step 4: virt-launcher Pod

Check the virt-launcher pod:
- Pod should be "Running" with all containers ready
- Check for container restart counts
- Review pod conditions for issues

${size(steps.pods.items) == 0 ? '*No virt-launcher pod found*' : '```yaml\n' + yaml(steps.pods.items.map(p, {'name': p.metadata.name, 'phase': has(p.status) && has(p.status.phase) ? p.status.phase : '', 'conditions': has(p.status) && has(p.status.conditions) ? p.status.conditions : [], 'containerStatuses': has(p.status) && has(p.status.containerStatuses) ? p.status.containerStatuses : []})) + '```'}

---

## Step 5: virt-launcher Pod Logs

Review the logs from the virt-launcher pod containers (secrets are redacted):
- Look for error messages or stack traces
- Check for VM boot issues or QEMU errors

${size(steps.pods.items) == 0 ? '*No virt-launcher pod found*' : range(size(steps.pods.items)).map(i, '### ' + steps.pods.items[i].metadata.name + '\n```\n' + (type(steps.logs[i]) == string ? steps.logs[i] : 'error fetching logs: ' + steps.logs[i].error) + '\n```').join('\n')}

---

## Step 6: Events

Review events related to the VM and its components:
- Look for Warning events
- Check for scheduling, storage, or network issues

${has(steps.events.error) ? '*Error listing events: ' + steps.events.error + '*' : (size(steps.events.items) == 0 ? '*No events found*' : '```yaml\n' + yaml(steps.events.items.map(e, {'type': e.type, 'reason': has(e.reason) ? e.reason : '', 'message': has(e.message) ? e.message : '', 'lastTimestamp': has(e.lastTimestamp) ? e.lastTimestamp : ''})) + '```')}

---

## Troubleshooting Analysis

Based on the data above, analyze:
1. **VM State**: Is the VM in the expected state (Running/Stopped)?
2. **VMI State**: Does the VMI exist and is it Running?
3. **Volumes**: Are all referenced volumes (DataVolumes, PVCs, ConfigMaps, Secrets, container images) available?
4. **Pod Health**: Is the virt-launcher pod running without restarts?
5. **Events**: Are there any Warning events indicating problems?

---

## Fix the Issue

If you identified a problem, attempt to fix it:
1. **Missing Resources**: Create missing ConfigMaps, Secrets, or PVCs referenced by the VM
2. **Storage Issues**: Check DataVolume import status, fix storage class or access mode issues
3. **Image Pull Errors**: Verify container disk image URLs are correct and accessible
4. **Configuration Errors**: Fix any syntax errors in cloud-init userData or other configurations
5. **Resource Constraints**: Adjust VM resource requests if the pod cannot be scheduled
6. **Restart VM**: If the VM is stuck, try stopping and starting it again

Use the available Kubernetes tools to apply fixes (create resources, update configurations, restart VM).

---

## Report Findings

After completing troubleshooting and attempting fixes, report:
- **Status:** Running/Stopped/Failed/Provisioning
- **Root Cause:** Description or "None found"
- **Action Taken:** What was done to fix the issue (or "None" if no fix was needed/possible)
- **Result:** Whether the fix was successful or further action is needed
