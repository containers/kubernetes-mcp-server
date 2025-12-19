package create

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"
)

//go:embed vm.yaml.tmpl
var vmYamlTemplate string

func Tools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "vm_create",
				Description: "Create a VirtualMachine in the cluster with the specified configuration, automatically resolving instance types, preferences, and container disk images. VM will be created in Halted state by default; use autostart parameter to start it immediately.",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace for the virtual machine",
						},
						"name": {
							Type:        "string",
							Description: "The name of the virtual machine",
						},
						"workload": {
							Type:        "string",
							Description: "The workload for the VM. Accepts OS names (e.g., 'fedora' (default), 'ubuntu', 'centos', 'centos-stream', 'debian', 'rhel', 'opensuse', 'opensuse-tumbleweed', 'opensuse-leap') or full container disk image URLs",
							Examples:    []any{"fedora", "ubuntu", "centos", "debian", "rhel", "quay.io/containerdisks/fedora:latest"},
						},
						"instancetype": {
							Type:        "string",
							Description: "Optional instance type name for the VM (e.g., 'u1.small', 'u1.medium', 'u1.large')",
						},
						"preference": {
							Type:        "string",
							Description: "Optional preference name for the VM",
						},
						"size": {
							Type:        "string",
							Description: "Optional workload size hint for the VM (e.g., 'small', 'medium', 'large', 'xlarge'). Used to auto-select an appropriate instance type if not explicitly specified.",
							Examples:    []any{"small", "medium", "large"},
						},
						"performance": {
							Type:        "string",
							Description: "Optional performance family hint for the VM instance type (e.g., 'u1' for general-purpose, 'o1' for overcommitted, 'c1' for compute-optimized, 'm1' for memory-optimized). Defaults to 'u1' (general-purpose) if not specified.",
							Examples:    []any{"general-purpose", "overcommitted", "compute-optimized", "memory-optimized"},
						},
						"autostart": {
							Type:        "boolean",
							Description: "Optional flag to automatically start the VM after creation (sets runStrategy to Always instead of Halted). Defaults to false.",
						},
						"storage": {
							Type:        "string",
							Description: "Optional storage size for the VM's root disk when using DataSources (e.g., '30Gi', '50Gi', '100Gi'). Defaults to 30Gi. Ignored when using container disks.",
							Examples:    []any{"30Gi", "50Gi", "100Gi"},
						},
					},
					Required: []string{"namespace", "name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Create",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: create,
		},
	}
}

type vmParams struct {
	Namespace           string
	Name                string
	ContainerDisk       string
	Instancetype        string
	InstancetypeKind    string
	Preference          string
	PreferenceKind      string
	UseDataSource       bool
	DataSourceName      string
	DataSourceNamespace string
	Storage             string
	RunStrategy         string
}

func create(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse and validate input parameters
	createParams, err := parseCreateParameters(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	dynamicClient := params.DynamicClient()

	// Search for available DataSources
	dataSources := kubevirt.SearchDataSources(params.Context, dynamicClient)

	// Match DataSource based on workload input
	matchedDataSource := kubevirt.MatchDataSource(dataSources, createParams.Workload)

	// Search for preferences and instancetypes
	preferences := kubevirt.SearchPreferences(params.Context, dynamicClient, createParams.Namespace)
	instancetypes := kubevirt.SearchInstancetypes(params.Context, dynamicClient, createParams.Namespace)

	// Resolve preference from DataSource defaults or cluster resources
	preferenceInfo := kubevirt.ResolvePreference(preferences, createParams.Preference, createParams.Workload, matchedDataSource)

	// Resolve instancetype from DataSource defaults or size/performance hints
	instancetypeInfo := kubevirt.ResolveInstancetype(instancetypes, createParams.Instancetype, createParams.Size, createParams.Performance, matchedDataSource)

	// Build template parameters from resolved resources
	templateParams := buildTemplateParams(createParams, matchedDataSource, instancetypeInfo, preferenceInfo)

	// Render the VM YAML
	vmYaml, err := renderVMYaml(templateParams)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	// Create the VM in the cluster
	resources, err := kubernetes.NewCore(params).ResourcesCreateOrUpdate(params, vmYaml)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create VirtualMachine: %w", err)), nil
	}

	// Format the output
	marshalledYaml, err := output.MarshalYaml(resources)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal created VirtualMachine: %w", err)), nil
	}

	return api.NewToolCallResult("# VirtualMachine created successfully\n"+marshalledYaml, nil), nil
}

// createParameters holds parsed input parameters for VM creation
type createParameters struct {
	Namespace    string
	Name         string
	Workload     string
	Instancetype string
	Preference   string
	Size         string
	Performance  string
	Storage      string
	Autostart    bool
}

// parseCreateParameters parses and validates input parameters
func parseCreateParameters(params api.ToolHandlerParams) (*createParameters, error) {
	namespace, err := api.RequiredString(params, "namespace")
	if err != nil {
		return nil, err
	}

	name, err := api.RequiredString(params, "name")
	if err != nil {
		return nil, err
	}

	return &createParameters{
		Namespace:    namespace,
		Name:         name,
		Workload:     api.OptionalString(params, "workload", "fedora"),
		Instancetype: api.OptionalString(params, "instancetype", ""),
		Preference:   api.OptionalString(params, "preference", ""),
		Size:         api.OptionalString(params, "size", ""),
		Performance:  normalizePerformance(api.OptionalString(params, "performance", "")),
		Storage:      api.OptionalString(params, "storage", "30Gi"),
		Autostart:    api.OptionalBool(params, "autostart", false),
	}, nil
}

// buildTemplateParams constructs the template parameters for VM creation
func buildTemplateParams(createParams *createParameters, matchedDataSource *kubevirt.DataSourceInfo, instancetypeInfo *kubevirt.InstancetypeInfo, preferenceInfo *kubevirt.PreferenceInfo) vmParams {
	// Determine runStrategy based on autostart parameter
	runStrategy := "Halted"
	if createParams.Autostart {
		runStrategy = "Always"
	}

	params := vmParams{
		Namespace:   createParams.Namespace,
		Name:        createParams.Name,
		Storage:     createParams.Storage,
		RunStrategy: runStrategy,
	}

	// Set instancetype and kind if available
	if instancetypeInfo != nil {
		params.Instancetype = instancetypeInfo.Name
		if instancetypeInfo.Namespace == "" {
			params.InstancetypeKind = "VirtualMachineClusterInstancetype"
		} else {
			params.InstancetypeKind = "VirtualMachineInstancetype"
		}
	}

	// Set preference and kind if available
	if preferenceInfo != nil {
		params.Preference = preferenceInfo.Name
		if preferenceInfo.Namespace == "" {
			params.PreferenceKind = "VirtualMachineClusterPreference"
		} else {
			params.PreferenceKind = "VirtualMachinePreference"
		}
	}

	if matchedDataSource != nil && matchedDataSource.Namespace != "" {
		// Use the matched DataSource (real cluster DataSource with namespace)
		params.UseDataSource = true
		params.DataSourceName = matchedDataSource.Name
		params.DataSourceNamespace = matchedDataSource.Namespace
	} else if matchedDataSource != nil {
		// Matched a built-in containerdisk (no namespace)
		params.ContainerDisk = matchedDataSource.Source
	} else {
		// No match, resolve container disk image from workload name
		params.ContainerDisk = resolveContainerDisk(createParams.Workload)
	}

	return params
}

// renderVMYaml renders the VM YAML from template
func renderVMYaml(templateParams vmParams) (string, error) {
	tmpl, err := template.New("vm").Parse(vmYamlTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var result strings.Builder
	if err := tmpl.Execute(&result, templateParams); err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	return result.String(), nil
}

func normalizePerformance(performance string) string {
	// Normalize to lowercase and trim spaces
	normalized := strings.ToLower(strings.TrimSpace(performance))

	// Map natural language terms to instance type prefixes
	performanceMap := map[string]string{
		"general-purpose":   "u1",
		"generalpurpose":    "u1",
		"general":           "u1",
		"overcommitted":     "o1",
		"compute":           "c1",
		"compute-optimized": "c1",
		"computeoptimized":  "c1",
		"memory-optimized":  "m1",
		"memoryoptimized":   "m1",
		"memory":            "m1",
		"u1":                "u1",
		"o1":                "o1",
		"c1":                "c1",
		"m1":                "m1",
	}

	// Look up the mapping
	if prefix, exists := performanceMap[normalized]; exists {
		return prefix
	}

	// Default to "u1" (general-purpose) if not recognized or empty
	return "u1"
}

// resolveContainerDisk resolves OS names to container disk images from quay.io/containerdisks
func resolveContainerDisk(input string) string {
	// If input already looks like a container image, return as-is
	if strings.Contains(input, "/") || strings.Contains(input, ":") {
		return input
	}

	// Common OS name mappings to containerdisk images
	osMap := map[string]string{
		"fedora":              "quay.io/containerdisks/fedora:latest",
		"ubuntu":              "quay.io/containerdisks/ubuntu:24.04",
		"centos":              "quay.io/containerdisks/centos-stream:9-latest",
		"centos-stream":       "quay.io/containerdisks/centos-stream:9-latest",
		"debian":              "quay.io/containerdisks/debian:latest",
		"opensuse":            "quay.io/containerdisks/opensuse-tumbleweed:1.0.0",
		"opensuse-tumbleweed": "quay.io/containerdisks/opensuse-tumbleweed:1.0.0",
		"opensuse-leap":       "quay.io/containerdisks/opensuse-leap:15.6",
		"rhel8":               "registry.redhat.io/rhel8/rhel-guest-image:latest",
		"rhel9":               "registry.redhat.io/rhel9/rhel-guest-image:latest",
		"rhel10":              "registry.redhat.io/rhel10/rhel-guest-image:latest",
	}

	// Normalize input to lowercase for lookup
	normalized := strings.ToLower(strings.TrimSpace(input))

	// Look up the OS name
	if containerDisk, exists := osMap[normalized]; exists {
		return containerDisk
	}

	// If no match found, return the input as-is (assume it's a valid container image URL)
	return input
}
