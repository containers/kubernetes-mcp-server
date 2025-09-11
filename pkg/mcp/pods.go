package mcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/kubectl/pkg/metricsutil"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

func (s *Server) initPods() []ServerTool {
	return []ServerTool{
		{Tool: Tool{
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
				},
			},
			Annotations: ToolAnnotations{
				Title:           "Pods: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: s.podsListInAllNamespaces},
		{Tool: Tool{
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
				},
				Required: []string{"namespace"},
			},
			Annotations: ToolAnnotations{
				Title:           "Pods: List in Namespace",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: s.podsListInNamespace},
		{Tool: Tool{
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
			Annotations: ToolAnnotations{
				Title:           "Pods: Get",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: s.podsGet},
		{Tool: Tool{
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
			Annotations: ToolAnnotations{
				Title:           "Pods: Delete",
				ReadOnlyHint:    ptr.To(false),
				DestructiveHint: ptr.To(true),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: s.podsDelete},
		{Tool: Tool{
			Name:        "pods_top",
			Description: "List the resource consumption (CPU and memory) as recorded by the Kubernetes Metrics Server for the specified Kubernetes Pods in the all namespaces, the provided namespace, or the current namespace",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"all_namespaces": {
						Type:        "boolean",
						Description: "If true, list the resource consumption for all Pods in all namespaces. If false, list the resource consumption for Pods in the provided namespace or the current namespace",
						Default:     ToRawMessage(true),
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
			Annotations: ToolAnnotations{
				Title:           "Pods: Top",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: s.podsTop},
		{Tool: Tool{
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
			Annotations: ToolAnnotations{
				Title:           "Pods: Exec",
				ReadOnlyHint:    ptr.To(false),
				DestructiveHint: ptr.To(true), // Depending on the Pod's entrypoint, executing certain commands may kill the Pod
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: s.podsExec},
		{Tool: Tool{
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
					"previous": {
						Type:        "boolean",
						Description: "Return previous terminated container logs (Optional)",
					},
				},
				Required: []string{"name"},
			},
			Annotations: ToolAnnotations{
				Title:           "Pods: Log",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: s.podsLog},
		{Tool: Tool{
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
			Annotations: ToolAnnotations{
				Title:           "Pods: Run",
				ReadOnlyHint:    ptr.To(false),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: s.podsRun},
	}
}

func (s *Server) podsListInAllNamespaces(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	labelSelector := ctr.GetArguments()["labelSelector"]
	resourceListOptions := kubernetes.ResourceListOptions{
		AsTable: s.configuration.ListOutput.AsTable(),
	}
	if labelSelector != nil {
		resourceListOptions.LabelSelector = labelSelector.(string)
	}
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.PodsListInAllNamespaces(ctx, resourceListOptions)
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to list pods in all namespaces: %v", err)), nil
	}
	return NewTextResult(s.configuration.ListOutput.PrintObj(ret)), nil
}

func (s *Server) podsListInNamespace(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ns := ctr.GetArguments()["namespace"]
	if ns == nil {
		return NewTextResult("", errors.New("failed to list pods in namespace, missing argument namespace")), nil
	}
	resourceListOptions := kubernetes.ResourceListOptions{
		AsTable: s.configuration.ListOutput.AsTable(),
	}
	labelSelector := ctr.GetArguments()["labelSelector"]
	if labelSelector != nil {
		resourceListOptions.LabelSelector = labelSelector.(string)
	}
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.PodsListInNamespace(ctx, ns.(string), resourceListOptions)
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to list pods in namespace %s: %v", ns, err)), nil
	}
	return NewTextResult(s.configuration.ListOutput.PrintObj(ret)), nil
}

func (s *Server) podsGet(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ns := ctr.GetArguments()["namespace"]
	if ns == nil {
		ns = ""
	}
	name := ctr.GetArguments()["name"]
	if name == nil {
		return NewTextResult("", errors.New("failed to get pod, missing argument name")), nil
	}
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.PodsGet(ctx, ns.(string), name.(string))
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to get pod %s in namespace %s: %v", name, ns, err)), nil
	}
	return NewTextResult(output.MarshalYaml(ret)), nil
}

func (s *Server) podsDelete(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ns := ctr.GetArguments()["namespace"]
	if ns == nil {
		ns = ""
	}
	name := ctr.GetArguments()["name"]
	if name == nil {
		return NewTextResult("", errors.New("failed to delete pod, missing argument name")), nil
	}
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.PodsDelete(ctx, ns.(string), name.(string))
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to delete pod %s in namespace %s: %v", name, ns, err)), nil
	}
	return NewTextResult(ret, err), nil
}

func (s *Server) podsTop(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	podsTopOptions := kubernetes.PodsTopOptions{AllNamespaces: true}
	if v, ok := ctr.GetArguments()["namespace"].(string); ok {
		podsTopOptions.Namespace = v
	}
	if v, ok := ctr.GetArguments()["all_namespaces"].(bool); ok {
		podsTopOptions.AllNamespaces = v
	}
	if v, ok := ctr.GetArguments()["name"].(string); ok {
		podsTopOptions.Name = v
	}
	if v, ok := ctr.GetArguments()["label_selector"].(string); ok {
		podsTopOptions.LabelSelector = v
	}
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.PodsTop(ctx, podsTopOptions)
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to get pods top: %v", err)), nil
	}
	buf := new(bytes.Buffer)
	printer := metricsutil.NewTopCmdPrinter(buf)
	err = printer.PrintPodMetrics(ret.Items, true, true, false, "", true)
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to get pods top: %v", err)), nil
	}
	return NewTextResult(buf.String(), nil), nil
}

func (s *Server) podsExec(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ns := ctr.GetArguments()["namespace"]
	if ns == nil {
		ns = ""
	}
	name := ctr.GetArguments()["name"]
	if name == nil {
		return NewTextResult("", errors.New("failed to exec in pod, missing argument name")), nil
	}
	container := ctr.GetArguments()["container"]
	if container == nil {
		container = ""
	}
	commandArg := ctr.GetArguments()["command"]
	command := make([]string, 0)
	if _, ok := commandArg.([]interface{}); ok {
		for _, cmd := range commandArg.([]interface{}) {
			if _, ok := cmd.(string); ok {
				command = append(command, cmd.(string))
			}
		}
	} else {
		return NewTextResult("", errors.New("failed to exec in pod, invalid command argument")), nil
	}
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.PodsExec(ctx, ns.(string), name.(string), container.(string), command)
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to exec in pod %s in namespace %s: %v", name, ns, err)), nil
	} else if ret == "" {
		ret = fmt.Sprintf("The executed command in pod %s in namespace %s has not produced any output", name, ns)
	}
	return NewTextResult(ret, err), nil
}

func (s *Server) podsLog(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ns := ctr.GetArguments()["namespace"]
	if ns == nil {
		ns = ""
	}
	name := ctr.GetArguments()["name"]
	if name == nil {
		return NewTextResult("", errors.New("failed to get pod log, missing argument name")), nil
	}
	container := ctr.GetArguments()["container"]
	if container == nil {
		container = ""
	}
	previous := ctr.GetArguments()["previous"]
	var previousBool bool
	if previous != nil {
		previousBool = previous.(bool)
	}
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.PodsLog(ctx, ns.(string), name.(string), container.(string), previousBool)
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to get pod %s log in namespace %s: %v", name, ns, err)), nil
	} else if ret == "" {
		ret = fmt.Sprintf("The pod %s in namespace %s has not logged any message yet", name, ns)
	}
	return NewTextResult(ret, err), nil
}

func (s *Server) podsRun(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ns := ctr.GetArguments()["namespace"]
	if ns == nil {
		ns = ""
	}
	name := ctr.GetArguments()["name"]
	if name == nil {
		name = ""
	}
	image := ctr.GetArguments()["image"]
	if image == nil {
		return NewTextResult("", errors.New("failed to run pod, missing argument image")), nil
	}
	port := ctr.GetArguments()["port"]
	if port == nil {
		port = float64(0)
	}
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	resources, err := derived.PodsRun(ctx, ns.(string), name.(string), image.(string), int32(port.(float64)))
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to run pod %s in namespace %s: %v", name, ns, err)), nil
	}
	marshalledYaml, err := output.MarshalYaml(resources)
	if err != nil {
		err = fmt.Errorf("failed to run pod: %v", err)
	}
	return NewTextResult("# The following resources (YAML) have been created or updated successfully\n"+marshalledYaml, err), nil
}
