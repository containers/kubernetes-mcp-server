package main

import (
	"embed"
	"encoding/json"

	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/dynamic"
)

//go:embed templates/*.md
var templates embed.FS

func tmpl(name string) string {
	raw, err := templates.ReadFile("templates/" + name + ".md")
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// promptConversion describes how one in-tree prompt becomes a Prompt
// resource. Name, Title, Description and Arguments override the source
// contract (and define it for prompts with no in-tree counterpart).
type promptConversion struct {
	Name        string
	Title       string
	Description string
	Arguments   []dynamic.PromptArgument
	Steps       []dynamic.Step
	Messages    []dynamic.PromptMessage
	Note        string
}

const analyzeVM = "I'll analyze the collected data to diagnose the VirtualMachine issues systematically."

func kubevirtPrompts() (map[string][]promptConversion, []promptConversion, map[string]string) {
	podLogs := dynamic.Step{Name: "logs", ForEach: "steps.pods.items", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{
		Verb: "get", APIVersion: "v1", Kind: "Pod", Namespace: nsArg(), Name: expr("item.metadata.name"), Subresource: val("log"),
		Query: map[string]dynamic.Ref{"tailLines": {Value: "100"}}, Response: dynamic.ResponseText, Redact: true}}
	windowsDefaults := "{'10': {'preferenceName': 'windows.10.virtio', 'autounattendConfigMapName': 'windows10-efi-autounattend', 'baseDvName': 'win10', 'isoDvName': 'win10', 'generateName': 'windows10-installer-run-'}, " +
		"'11': {'preferenceName': 'windows.11.virtio', 'autounattendConfigMapName': 'windows11-autounattend', 'baseDvName': 'win11', 'isoDvName': 'win11', 'generateName': 'windows11-installer-run-'}, " +
		"'2k22': {'preferenceName': 'windows.2k22.virtio', 'autounattendConfigMapName': 'windows2k22-autounattend', 'baseDvName': 'win2k22', 'isoDvName': 'win2k22', 'generateName': 'windows2k22-installer-run-'}, " +
		"'2k25': {'preferenceName': 'windows.2k25.virtio', 'autounattendConfigMapName': 'windows2k25-autounattend', 'baseDvName': 'win2k25', 'isoDvName': 'win2k25', 'generateName': 'windows2k25-installer-run-'}}[steps.version]"
	pipelineRun := "{'apiVersion': 'tekton.dev/v1', 'kind': 'PipelineRun', " +
		"'metadata': merge({'generateName': steps.defaults.generateName}, has(args.namespace) && args.namespace != '' ? {'namespace': args.namespace} : {}), " +
		"'spec': {'params': [{'name': 'winImageDownloadURL', 'value': args.winImageDownloadURL}, {'name': 'acceptEula', 'value': steps.eula.action == 'accept' ? 'true' : 'false'}, " +
		"{'name': 'preferenceName', 'value': steps.defaults.preferenceName}, {'name': 'autounattendConfigMapName', 'value': steps.defaults.autounattendConfigMapName}, " +
		"{'name': 'baseDvName', 'value': steps.defaults.baseDvName}, {'name': 'isoDVName', 'value': steps.defaults.isoDvName}], " +
		"'pipelineRef': {'resolver': 'hub', 'params': [{'name': 'catalog', 'value': 'kubevirt-tekton-pipelines'}, {'name': 'type', 'value': 'artifact'}, {'name': 'kind', 'value': 'pipeline'}, {'name': 'name', 'value': 'windows-efi-installer'}] + " +
		"(has(args.pipelineVersion) && args.pipelineVersion != '' ? [{'name': 'version', 'value': args.pipelineVersion}] : [])}, " +
		"'taskRunTemplate': {'podTemplate': {'securityContext': {'runAsUser': 107, 'fsGroup': 107}}}}}"
	ported := map[string][]promptConversion{
		"vm-troubleshoot": {{
			Steps: []dynamic.Step{
				{Name: "vm", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "get", APIVersion: "kubevirt.io/v1", Kind: "VirtualMachine", Namespace: nsArg(), Name: ref("name")}},
				{Name: "vmi", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "get", APIVersion: "kubevirt.io/v1", Kind: "VirtualMachineInstance", Namespace: nsArg(), Name: ref("name")}},
				{Name: "pods", Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "v1", Kind: "Pod", Namespace: nsArg(),
					Query: map[string]dynamic.Ref{"labelSelector": {Value: "kubevirt.io=virt-launcher,vm.kubevirt.io/name={name}"}}}},
				podLogs,
				{Name: "events", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "v1", Kind: "Event", Namespace: nsArg(),
					Query: map[string]dynamic.Ref{"fieldSelector": {Value: "involvedObject.name={name}"}}}},
			},
			Messages: []dynamic.PromptMessage{{Role: "user", Text: tmpl("vm-troubleshoot")}, {Role: "assistant", Text: analyzeVM}},
			Note:     "Pod logs are included with secrets redacted; cloud-init volumes are redacted wholesale instead of field by field.",
		}},
		"hco-status": {{
			Steps: []dynamic.Step{
				{Name: "hcos", Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "hco.kubevirt.io/v1", Kind: "HyperConverged", Namespace: &dynamic.NamespaceRef{All: true}}},
				{Name: "present", Fail: "size(steps.hcos.items) == 0 ? 'no HyperConverged resource found in the cluster' : ''"},
				{Name: "hco", Expression: "steps.hcos.items[0]"},
				{Name: "kubevirts", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "kubevirt.io/v1", Kind: "KubeVirt", Namespace: &dynamic.NamespaceRef{Expression: "steps.hco.metadata.namespace"}}},
				{Name: "cdis", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "cdi.kubevirt.io/v1beta1", Kind: "CDI"}},
				{Name: "nacs", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "networkaddonsoperator.network.kubevirt.io/v1", Kind: "NetworkAddonsConfig"}},
				{Name: "events", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "v1", Kind: "Event", Namespace: &dynamic.NamespaceRef{Expression: "steps.hco.metadata.namespace"}}},
			},
			Messages: []dynamic.PromptMessage{{Role: "user", Text: tmpl("hco-status")}, {Role: "assistant", Text: "I'll analyze the HyperConverged Cluster Operator status report to assess the overall health and configuration."}},
		}},
		"windows-golden-image": {{
			Steps: []dynamic.Step{
				{Name: "check", Fail: "!has(args.winImageDownloadURL) || !args.winImageDownloadURL.startsWith('https://') || args.winImageDownloadURL.contains('@') ? 'winImageDownloadURL must be an https:// URL without embedded credentials' : " +
					"(has(args.pipelineVersion) && args.pipelineVersion != '' && !args.pipelineVersion.matches('^\\\\d+\\\\.\\\\d+\\\\.\\\\d+$') ? 'invalid pipelineVersion: must be in format X.Y.Z (e.g. 0.25.0)' : '')"},
				{Name: "version", Expression: "has(args.windowsVersion) && args.windowsVersion != '' ? args.windowsVersion : '2k22'"},
				{Name: "supported", Fail: "!(steps.version in ['10', '11', '2k22', '2k25']) ? 'unsupported Windows version ' + steps.version + ': must be one of 10, 11, 2k22, 2k25' : ''"},
				{Name: "defaults", Expression: windowsDefaults},
				{Name: "eula", Elicit: &dynamic.ElicitStep{Message: "Microsoft EULA Notice: By proceeding, you agree to the Microsoft Software License Terms for the Windows operating system. Do you accept the Microsoft End User License Agreement (EULA)?"}},
				{Name: "pipelinerun", Expression: pipelineRun},
			},
			Messages: []dynamic.PromptMessage{
				{Role: "user", When: "steps.eula.action == 'accept'", Text: tmpl("windows-golden-image-accepted")},
				{Role: "user", When: "steps.eula.action == 'unsupported'", Text: tmpl("windows-golden-image-fallback")},
				{Role: "assistant", When: "steps.eula.action in ['decline', 'cancel']", Text: "The Microsoft EULA was not accepted. The Windows golden image creation has been cancelled."},
			},
			Note: "The Tekton Hub catalog name is fixed to the upstream default; downstream builds override it in Go.",
		}},
	}
	extra := []promptConversion{{
		Name:        "vm-create",
		Title:       "Virtual Machine: Create",
		Description: "Gather the DataSources, instancetypes, preferences and templates available for a new VirtualMachine so the right ones can be chosen for the requested workload, then create it. Replaces the vm_create tool's built-in matching heuristics.",
		Arguments: []dynamic.PromptArgument{
			{Name: "namespace", Description: "Namespace for the new VirtualMachine", Required: true},
			{Name: "name", Description: "Name of the new VirtualMachine", Required: true},
			{Name: "workload", Description: "Workload or OS to run (e.g. fedora, rhel9, windows)"},
			{Name: "size", Description: "Rough size: small, medium or large"},
			{Name: "performance", Description: "Performance family: general, compute or memory"},
			{Name: "instancetype", Description: "Explicit instancetype name"},
			{Name: "preference", Description: "Explicit preference name"},
			{Name: "autostart", Description: "Start the VM after creation (true/false)"},
		},
		Steps: []dynamic.Step{
			{Name: "templates", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "template.kubevirt.io/v1beta1", Kind: "VirtualMachineTemplate", Namespace: nsArg()}},
			{Name: "datasources", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "cdi.kubevirt.io/v1beta1", Kind: "DataSource", Namespace: &dynamic.NamespaceRef{All: true}}},
			{Name: "clusterPreferences", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "instancetype.kubevirt.io/v1beta1", Kind: "VirtualMachineClusterPreference"}},
			{Name: "preferences", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "instancetype.kubevirt.io/v1beta1", Kind: "VirtualMachinePreference", Namespace: nsArg()}},
			{Name: "clusterInstancetypes", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "instancetype.kubevirt.io/v1beta1", Kind: "VirtualMachineClusterInstancetype"}},
			{Name: "instancetypes", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "instancetype.kubevirt.io/v1beta1", Kind: "VirtualMachineInstancetype", Namespace: nsArg()}},
		},
		Messages: []dynamic.PromptMessage{{Role: "user", Text: tmpl("vm-create")}, {Role: "assistant", Text: "I'll pick the best matching template, boot image, instancetype and preference and create the VirtualMachine."}},
		Note:     "New: hands the model the candidates the vm_create tool matched heuristically.",
	}}
	return ported, extra, nil
}

func tektonPrompts() (map[string][]promptConversion, []promptConversion, map[string]string) {
	failed := "steps.runs.map(tr, !(has(tr.status) && has(tr.status.podName) && tr.status.podName != '' && has(tr.status.steps)) ? [] : " +
		"tr.status.steps.filter(st, (has(st.terminated) && ((has(st.terminated.exitCode) && st.terminated.exitCode != 0) || (has(st.terminated.reason) && st.terminated.reason != '' && st.terminated.reason != 'Completed'))) || " +
		"(has(st.waiting) && has(st.waiting.reason) && st.waiting.reason != '' && !(st.waiting.reason in ['PodInitializing', 'ContainerCreating'])))" +
		".map(st, {'taskRun': tr.metadata.name, 'pod': tr.status.podName, 'name': st.name, 'container': st.container})).flatten()"
	return map[string][]promptConversion{
		"pipeline-troubleshoot": {{
			Steps: []dynamic.Step{
				{Name: "pr", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "get", APIVersion: "tekton.dev/v1", Kind: "PipelineRun", Namespace: nsArg(), Name: ref("name")}},
				{Name: "pipeline", IgnoreError: true, When: "!has(steps.pr.error) && has(steps.pr.spec.pipelineRef) && has(steps.pr.spec.pipelineRef.name)",
					Kubernetes: &dynamic.KubernetesAction{Verb: "get", APIVersion: "tekton.dev/v1", Kind: "Pipeline", Namespace: nsArg(), Name: expr("steps.pr.spec.pipelineRef.name")}},
				{Name: "taskruns", Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "tekton.dev/v1", Kind: "TaskRun", Namespace: nsArg(),
					Query: map[string]dynamic.Ref{"labelSelector": {Value: "tekton.dev/pipelineRun={name}"}}}},
				{Name: "runs", Expression: "steps.taskruns.items"},
				{Name: "failed", Expression: failed},
				{Name: "logs", ForEach: "steps.failed", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{
					Verb: "get", APIVersion: "v1", Kind: "Pod", Namespace: nsArg(), Name: expr("item.pod"), Subresource: val("log"),
					Query: map[string]dynamic.Ref{"container": {Expression: "item.container"}, "tailLines": {Value: "100"}}, Response: dynamic.ResponseText, Redact: true}},
				{Name: "events", ForEach: "[args.name] + steps.runs.map(tr, tr.metadata.name) + steps.runs.filter(tr, has(tr.status) && has(tr.status.podName) && tr.status.podName != '').map(tr, tr.status.podName)", IgnoreError: true,
					Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "v1", Kind: "Event", Namespace: nsArg(), Query: map[string]dynamic.Ref{"fieldSelector": {Expression: "'involvedObject.name=' + item"}}}},
				{Name: "warnings", Expression: "steps.events.map(e, has(e.error) ? [] : e.items.filter(ev, ev.type == 'Warning').map(ev, {'object': ev.involvedObject.kind + '/' + ev.involvedObject.name, 'reason': has(ev.reason) ? ev.reason : '', 'message': has(ev.message) ? ev.message : '', 'lastTimestamp': has(ev.lastTimestamp) ? ev.lastTimestamp : ''})).flatten()"},
				{Name: "pac", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "pipelinesascode.tekton.dev/v1alpha1", Kind: "Repository", Namespace: nsArg()}},
				{Name: "tektonconfigs", IgnoreError: true, Kubernetes: &dynamic.KubernetesAction{Verb: "list", APIVersion: "operator.tekton.dev/v1alpha1", Kind: "TektonConfig"}},
			},
			Messages: []dynamic.PromptMessage{{Role: "user", Text: tmpl("pipeline-troubleshoot")}, {Role: "assistant", Text: "I'll analyze the collected Tekton data to identify the PipelineRun issue."}},
			Note:     "Failed-step logs are redacted; the TaskRun section is a projection of the built-in text table.",
		}},
	}, nil, nil
}

// promptManifest is the Prompt resource written to the chart.
type promptManifest struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels,omitempty"`
	} `json:"metadata"`
	Spec dynamic.PromptSpec `json:"spec"`
}

func promptManifestOf(toolsetName string, c promptConversion) (promptManifest, error) {
	var m promptManifest
	m.APIVersion = dynamic.Group + "/" + dynamic.Version
	m.Kind = dynamic.PromptKind
	m.Spec = dynamic.PromptSpec{
		Name: c.Name, Title: c.Title, Description: c.Description,
		Arguments: c.Arguments, Steps: c.Steps, Messages: c.Messages,
	}
	m.Metadata.Name = c.Name
	m.Metadata.Labels = map[string]string{
		"app.kubernetes.io/part-of":                     "kubernetes-mcp-server",
		"tools.kubernetes-mcp-server.io/source-toolset": toolsetName,
	}
	// Validate exactly what the server will load, with the Helm placeholders
	// replaced by legal values (deep copy through JSON).
	raw, err := json.Marshal(m.Spec)
	if err != nil {
		return m, err
	}
	var check dynamic.PromptSpec
	if err := json.Unmarshal(raw, &check); err != nil {
		return m, err
	}
	for i := range check.Steps {
		if h := check.Steps[i].HTTP; h != nil && h.URL == urlSentinel {
			h.URL = "https://placeholder.invalid"
			h.CertificateAuthority = ""
		}
	}
	if err := dynamic.CheckPrompt(dynamic.Prompt{Metadata: dynamic.Metadata{Name: m.Metadata.Name}, Spec: check}); err != nil {
		return m, err
	}
	return m, nil
}
