package core

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/kubectl/pkg/metricsutil"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/mcplog"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

func initPods() []api.ServerTool {
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "pods_list",
			Description: "List all the Kubernetes pods in the current cluster from all namespaces",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"labelSelector": {
						Type:        "string",
						Description: "Optional Kubernetes label selector (e.g. 'app=myapp,env=prod' or 'app in (myapp,yourapp)'), use this option when you want to filter the pods by label",
						Pattern:     "([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]",
					},
					"fieldSelector": {
						Type:        "string",
						Description: "Optional Kubernetes field selector to filter pods by field values (e.g. 'status.phase=Running', 'spec.nodeName=node1'). Supported fields: metadata.name, metadata.namespace, spec.nodeName, spec.restartPolicy, spec.schedulerName, spec.serviceAccountName, status.phase (Pending/Running/Succeeded/Failed/Unknown), status.podIP, status.nominatedNodeName. Note: CrashLoopBackOff is a container state, not a pod phase, so it cannot be filtered directly. See https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/",
						Pattern:     "([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]",
					},
				},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Pods: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: podsListInAllNamespaces},
		{Tool: api.Tool{
			Name:        "pods_list_in_namespace",
			Description: "List all the Kubernetes pods in the specified namespace in the current cluster",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace to list pods from",
					},
					"labelSelector": {
						Type:        "string",
						Description: "Optional Kubernetes label selector (e.g. 'app=myapp,env=prod' or 'app in (myapp,yourapp)'), use this option when you want to filter the pods by label",
						Pattern:     "([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]",
					},
					"fieldSelector": {
						Type:        "string",
						Description: "Optional Kubernetes field selector to filter pods by field values (e.g. 'status.phase=Running', 'spec.nodeName=node1'). Supported fields: metadata.name, metadata.namespace, spec.nodeName, spec.restartPolicy, spec.schedulerName, spec.serviceAccountName, status.phase (Pending/Running/Succeeded/Failed/Unknown), status.podIP, status.nominatedNodeName. Note: CrashLoopBackOff is a container state, not a pod phase, so it cannot be filtered directly. See https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/",
						Pattern:     "([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]",
					},
				},
				Required: []string{"namespace"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Pods: List in Namespace",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: podsListInNamespace},
		{Tool: api.Tool{
			Name:        "pods_get",
			Description: "Get a Kubernetes Pod in the current or provided namespace with the provided name",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace to get the Pod from",
					},
					"name": {
						Type:        "string",
						Description: "Name of the Pod",
					},
				},
				Required: []string{"name"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Pods: Get",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: podsGet},
		{Tool: api.Tool{
			Name:        "pods_delete",
			Description: "Delete a Kubernetes Pod in the current or provided namespace with the provided name",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace to delete the Pod from",
					},
					"name": {
						Type:        "string",
						Description: "Name of the Pod to delete",
					},
				},
				Required: []string{"name"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Pods: Delete",
				DestructiveHint: ptr.To(true),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: podsDelete},
		{Tool: api.Tool{
			Name:        "pods_top",
			Description: "List the resource consumption (CPU and memory) as recorded by the Kubernetes Metrics Server for the specified Kubernetes Pods in the all namespaces, the provided namespace, or the current namespace",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"all_namespaces": {
						Type:        "boolean",
						Description: "If true, list the resource consumption for all Pods in all namespaces. If false, list the resource consumption for Pods in the provided namespace or the current namespace",
						Default:     api.ToRawMessage(true),
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace to get the Pods resource consumption from (Optional, current namespace if not provided and all_namespaces is false)",
					},
					"name": {
						Type:        "string",
						Description: "Name of the Pod to get the resource consumption from (Optional, all Pods in the namespace if not provided)",
					},
					"label_selector": {
						Type:        "string",
						Description: "Kubernetes label selector (e.g. 'app=myapp,env=prod' or 'app in (myapp,yourapp)'), use this option when you want to filter the pods by label (Optional, only applicable when name is not provided)",
						Pattern:     "([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]",
					},
				},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Pods: Top",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: podsTop},
		{Tool: api.Tool{
			Name:        "pods_exec",
			Description: "Execute a command in a Kubernetes Pod in the current or provided namespace with the provided name and command",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace of the Pod where the command will be executed",
					},
					"name": {
						Type:        "string",
						Description: "Name of the Pod where the command will be executed",
					},
					"command": {
						Type:        "array",
						Description: "Command to execute in the Pod container. The first item is the command to be run, and the rest are the arguments to that command. Example: [\"ls\", \"-l\", \"/tmp\"]",
						Items: &jsonschema.Schema{
							Type: "string",
						},
					},
					"container": {
						Type:        "string",
						Description: "Name of the Pod container where the command will be executed (Optional)",
					},
				},
				Required: []string{"name", "command"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Pods: Exec",
				DestructiveHint: ptr.To(true), // Depending on the Pod's entrypoint, executing certain commands may kill the Pod
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: podsExec},
		{Tool: api.Tool{
			Name:        "pods_log",
			Description: "Get the logs of a Kubernetes Pod in the current or provided namespace with the provided name",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace to get the Pod logs from",
					},
					"name": {
						Type:        "string",
						Description: "Name of the Pod to get the logs from",
					},
					"container": {
						Type:        "string",
						Description: "Name of the Pod container to get the logs from (Optional)",
					},
					"tail": {
						Type:        "integer",
						Description: "Number of lines to retrieve from the end of the logs (Optional, default: 100)",
						Default:     api.ToRawMessage(kubernetes.DefaultTailLines),
						Minimum:     ptr.To(float64(0)),
					},
					"previous": {
						Type:        "boolean",
						Description: "Return previous terminated container logs (Optional)",
					},
				},
				Required: []string{"name"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Pods: Log",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: podsLog},
		{Tool: api.Tool{
			Name:        "pods_run",
			Description: "Run a Kubernetes Pod in the current or provided namespace with the provided container image and optional name",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace to run the Pod in",
					},
					"name": {
						Type:        "string",
						Description: "Name of the Pod (Optional, random name if not provided)",
					},
					"image": {
						Type:        "string",
						Description: "Container Image to run in the Pod",
					},
					"port": {
						Type:        "number",
						Description: "TCP/IP port to expose from the Pod container (Optional, no port exposed if not provided)",
					},
				},
				Required: []string{"image"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Pods: Run",
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: podsRun},
	}
}

func podsListInAllNamespaces(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	labelSelector := params.GetArguments()["labelSelector"]
	fieldSelector := params.GetArguments()["fieldSelector"]
	resourceListOptions := api.ListOptions{
		AsTable: params.ListOutput.AsTable(),
	}
	if labelSelector != nil {
		resourceListOptions.LabelSelector = labelSelector.(string)
	}
	if fieldSelector != nil {
		resourceListOptions.FieldSelector = fieldSelector.(string)
	}
	ret, err := kubernetes.NewCore(params).PodsListInAllNamespaces(params, resourceListOptions)
	if err != nil {
		mcplog.HandleK8sError(params.Context, err, "pod listing")
		return api.NewToolCallResult("", fmt.Errorf("failed to list pods in all namespaces: %w", err)), nil
	}
	return api.NewToolCallResult(params.ListOutput.PrintObj(ret)), nil
}

func podsListInNamespace(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	ns := params.GetArguments()["namespace"]
	if ns == nil {
		return api.NewToolCallResult("", errors.New("failed to list pods in namespace, missing argument namespace")), nil
	}
	resourceListOptions := api.ListOptions{
		AsTable: params.ListOutput.AsTable(),
	}
	labelSelector := params.GetArguments()["labelSelector"]
	if labelSelector != nil {
		resourceListOptions.LabelSelector = labelSelector.(string)
	}
	fieldSelector := params.GetArguments()["fieldSelector"]
	if fieldSelector != nil {
		resourceListOptions.FieldSelector = fieldSelector.(string)
	}
	ret, err := kubernetes.NewCore(params).PodsListInNamespace(params, ns.(string), resourceListOptions)
	if err != nil {
		mcplog.HandleK8sError(params.Context, err, "pod listing")
		return api.NewToolCallResult("", fmt.Errorf("failed to list pods in namespace %s: %w", ns, err)), nil
	}
	return api.NewToolCallResult(params.ListOutput.PrintObj(ret)), nil
}

func podsGet(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	ns := params.GetArguments()["namespace"]
	if ns == nil {
		ns = ""
	}
	name := params.GetArguments()["name"]
	if name == nil {
		return api.NewToolCallResult("", errors.New("failed to get pod, missing argument name")), nil
	}
	ret, err := kubernetes.NewCore(params).PodsGet(params, ns.(string), name.(string))
	if err != nil {
		mcplog.HandleK8sError(params.Context, err, "pod access")
		return api.NewToolCallResult("", fmt.Errorf("failed to get pod %s in namespace %s: %w", name, ns, err)), nil
	}
	return api.NewToolCallResult(output.MarshalYaml(ret)), nil
}

func podsDelete(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	ns := params.GetArguments()["namespace"]
	if ns == nil {
		ns = ""
	}
	name := params.GetArguments()["name"]
	if name == nil {
		return api.NewToolCallResult("", errors.New("failed to delete pod, missing argument name")), nil
	}
	ret, err := kubernetes.NewCore(params).PodsDelete(params, ns.(string), name.(string))
	if err != nil {
		mcplog.HandleK8sError(params.Context, err, "pod deletion")
		return api.NewToolCallResult("", fmt.Errorf("failed to delete pod %s in namespace %s: %w", name, ns, err)), nil
	}
	return api.NewToolCallResult(ret, err), nil
}

func podsTop(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	podsTopOptions := api.PodsTopOptions{AllNamespaces: true}
	if v, ok := params.GetArguments()["namespace"].(string); ok {
		podsTopOptions.Namespace = v
	}
	if v, ok := params.GetArguments()["all_namespaces"].(bool); ok {
		podsTopOptions.AllNamespaces = v
	}
	if v, ok := params.GetArguments()["name"].(string); ok {
		podsTopOptions.Name = v
	}
	if v, ok := params.GetArguments()["label_selector"].(string); ok {
		podsTopOptions.LabelSelector = v
	}
	ret, err := kubernetes.NewCore(params).PodsTop(params, podsTopOptions)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get pods top: %w", err)), nil
	}
	buf := new(bytes.Buffer)
	printer := metricsutil.NewTopCmdPrinter(buf, true)
	err = printer.PrintPodMetrics(ret.Items, true, true, false, "", true)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get pods top: %w", err)), nil
	}
	return api.NewToolCallResult(buf.String(), nil), nil
}

func podsExec(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	ns := params.GetArguments()["namespace"]
	if ns == nil {
		ns = ""
	}
	name := params.GetArguments()["name"]
	if name == nil {
		return api.NewToolCallResult("", errors.New("failed to exec in pod, missing argument name")), nil
	}
	container := params.GetArguments()["container"]
	if container == nil {
		container = ""
	}
	commandArg := params.GetArguments()["command"]
	command := make([]string, 0)
	if _, ok := commandArg.([]interface{}); ok {
		for _, cmd := range commandArg.([]interface{}) {
			if _, ok := cmd.(string); ok {
				command = append(command, cmd.(string))
			}
		}
	} else {
		return api.NewToolCallResult("", errors.New("failed to exec in pod, invalid command argument")), nil
	}
	ret, err := kubernetes.NewCore(params).PodsExec(params, ns.(string), name.(string), container.(string), command)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to exec in pod %s in namespace %s: %w", name, ns, err)), nil
	} else if ret == "" {
		ret = fmt.Sprintf("The executed command in pod %s in namespace %s has not produced any output", name, ns)
	}
	return api.NewToolCallResult(ret, err), nil
}

func podsLog(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	ns := params.GetArguments()["namespace"]
	if ns == nil {
		ns = ""
	}
	name := params.GetArguments()["name"]
	if name == nil {
		return api.NewToolCallResult("", errors.New("failed to get pod log, missing argument name")), nil
	}
	container := params.GetArguments()["container"]
	if container == nil {
		container = ""
	}
	previous := params.GetArguments()["previous"]
	var previousBool bool
	if previous != nil {
		previousBool = previous.(bool)
	}
	// Extract tailLines parameter
	tail := params.GetArguments()["tail"]
	var tailInt int64
	if tail != nil {
		var err error
		tailInt, err = api.ParseInt64(tail)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to parse tail parameter: %w", err)), nil
		}
	}

	ret, err := kubernetes.NewCore(params).PodsLog(params.Context, ns.(string), name.(string), container.(string), previousBool, tailInt)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get pod %s log in namespace %s: %w", name, ns, err)), nil
	} else if ret == "" {
		ret = fmt.Sprintf("The pod %s in namespace %s has not logged any message yet", name, ns)
	}
	return api.NewToolCallResult(ret, err), nil
}

func podsRun(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	ns := params.GetArguments()["namespace"]
	if ns == nil {
		ns = ""
	}
	name := params.GetArguments()["name"]
	if name == nil {
		name = ""
	}
	image := params.GetArguments()["image"]
	if image == nil {
		return api.NewToolCallResult("", errors.New("failed to run pod, missing argument image")), nil
	}
	port := params.GetArguments()["port"]
	if port == nil {
		port = float64(0)
	}
	resources, err := kubernetes.NewCore(params).PodsRun(params, ns.(string), name.(string), image.(string), int32(port.(float64)))
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to run pod %s in namespace %s: %w", name, ns, err)), nil
	}
	marshalledYaml, err := output.MarshalYaml(resources)
	if err != nil {
		err = fmt.Errorf("failed to run pod: %w", err)
	}
	return api.NewToolCallResult("# The following resources (YAML) have been created or updated successfully\n"+marshalledYaml, err), nil
}
