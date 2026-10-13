# Create the KubeVirt VirtualMachine ${args.namespace}/${args.name}

Requested: workload `${has(args.workload) ? args.workload : 'not specified'}`, size `${has(args.size) ? args.size : 'not specified'}`, performance `${has(args.performance) ? args.performance : 'not specified'}`${has(args.instancetype) && args.instancetype != '' ? ', instancetype `' + args.instancetype + '`' : ''}${has(args.preference) && args.preference != '' ? ', preference `' + args.preference + '`' : ''}.

The candidates available in the cluster are listed below. Choose the best match for the request, then create the VM.

## VirtualMachineTemplates in ${args.namespace}

${has(steps.templates.error) ? '*Templates unavailable: ' + steps.templates.error + '*' : (size(steps.templates.items) == 0 ? '*No VirtualMachineTemplates in this namespace*' : '```yaml\n' + yaml(steps.templates.items.map(t, {'name': t.metadata.name, 'description': has(t.metadata.annotations) && 'description' in t.metadata.annotations ? t.metadata.annotations['description'] : '', 'parameters': has(t.spec) && has(t.spec.parameters) ? t.spec.parameters.map(p, p.name) : []})) + '```')}

## DataSources (boot images)

${has(steps.datasources.error) ? '*DataSources unavailable: ' + steps.datasources.error + '*' : (size(steps.datasources.items) == 0 ? '*No DataSources found*' : '```yaml\n' + yaml(steps.datasources.items.map(d, {'namespace': d.metadata.namespace, 'name': d.metadata.name, 'labels': has(d.metadata.labels) ? d.metadata.labels : {}})) + '```')}

## Preferences

${'```yaml\n' + yaml({'cluster': has(steps.clusterPreferences.error) ? [] : steps.clusterPreferences.items.map(p, p.metadata.name), 'namespaced': has(steps.preferences.error) ? [] : steps.preferences.items.map(p, p.metadata.name)}) + '```'}

## Instancetypes

${'```yaml\n' + yaml({'cluster': has(steps.clusterInstancetypes.error) ? [] : steps.clusterInstancetypes.items.map(i, {'name': i.metadata.name, 'class': has(i.metadata.labels) && 'instancetype.kubevirt.io/class' in i.metadata.labels ? i.metadata.labels['instancetype.kubevirt.io/class'] : '', 'cpu': has(i.spec) && has(i.spec.cpu) ? i.spec.cpu : {}, 'memory': has(i.spec) && has(i.spec.memory) ? i.spec.memory : {}}), 'namespaced': has(steps.instancetypes.error) ? [] : steps.instancetypes.items.map(i, i.metadata.name)}) + '```'}

## How to proceed

1. If a VirtualMachineTemplate matches the workload, prefer `vm_create_from_template` with that template and the requested name and parameters.
2. Otherwise pick the DataSource whose name matches the workload, the preference named after the same OS, and an instancetype whose class and size fit the request (`u1` general purpose, `cx1` compute, `m1` memory; `small`/`medium`/`large`), then create this VirtualMachine with `resources_create_or_update`:

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: ${args.name}
  namespace: ${args.namespace}
spec:
  runStrategy: ${has(args.autostart) && args.autostart == 'true' ? 'Always' : 'Halted'}
  instancetype:
    name: <chosen instancetype>
  preference:
    name: <chosen preference>
  dataVolumeTemplates:
    - metadata:
        name: ${args.name}-root
      spec:
        sourceRef:
          kind: DataSource
          name: <chosen DataSource name>
          namespace: <chosen DataSource namespace>
        storage: {}
  template:
    spec:
      domain:
        devices: {}
      volumes:
        - name: root
          dataVolume:
            name: ${args.name}-root
```

Report which candidates were chosen and why.
