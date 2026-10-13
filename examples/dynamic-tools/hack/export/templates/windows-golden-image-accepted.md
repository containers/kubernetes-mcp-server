# KubeVirt Windows Golden Image Creator

The user has accepted the Microsoft EULA.

## PipelineRun YAML

The following PipelineRun will:
1. Download the Windows ISO from the provided URL
2. Modify it for EFI automated installation
3. Create a VM and install Windows
4. Produce a bootable DataSource/DataVolume as a golden image

```yaml
${yaml(steps.pipelinerun)}```

## Next steps

1. Create the PipelineRun above with `resources_create_or_update` in namespace `${has(args.namespace) && args.namespace != '' ? args.namespace : 'the current namespace'}`.
2. Follow its progress with `tekton_pipelinerun_logs` and `tekton_pipelinerun_lifecycle`.
3. When it succeeds, the golden image is available as the `${steps.defaults.baseDvName}` DataSource for new VirtualMachines.
