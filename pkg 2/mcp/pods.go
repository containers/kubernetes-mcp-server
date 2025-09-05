package mcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/kubectl/pkg/metricsutil"

	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

func (s *Server) initPods() []server.ServerTool {
	return []server.ServerTool{
		{Tool: mcp.NewTool("pods_list",
			mcp.WithDescription("List all the Kubernetes pods in the current cluster from all namespaces"),
			mcp.WithString("labelSelector", mcp.Description("Optional Kubernetes label selector (e.g. 'app=myapp,env=prod' or 'app in (myapp,yourapp)'), use this option when you want to filter the pods by label"), mcp.Pattern("([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]")),
			// Tool annotations
			mcp.WithTitleAnnotation("Pods: List"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.podsListInAllNamespaces},
		{Tool: mcp.NewTool("pods_list_in_namespace",
			mcp.WithDescription("List all the Kubernetes pods in the specified namespace in the current cluster"),
			mcp.WithString("namespace", mcp.Description("Namespace to list pods from"), mcp.Required()),
			mcp.WithString("labelSelector", mcp.Description("Optional Kubernetes label selector (e.g. 'app=myapp,env=prod' or 'app in (myapp,yourapp)'), use this option when you want to filter the pods by label"), mcp.Pattern("([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]")),
			// Tool annotations
			mcp.WithTitleAnnotation("Pods: List in Namespace"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.podsListInNamespace},
		{Tool: mcp.NewTool("pods_get",
			mcp.WithDescription("Get a Kubernetes Pod in the current or provided namespace with the provided name"),
			mcp.WithString("namespace", mcp.Description("Namespace to get the Pod from")),
			mcp.WithString("name", mcp.Description("Name of the Pod"), mcp.Required()),
			// Tool annotations
			mcp.WithTitleAnnotation("Pods: Get"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.podsGet},
		{Tool: mcp.NewTool("pods_delete",
			mcp.WithDescription("Delete a Kubernetes Pod in the current or provided namespace with the provided name"),
			mcp.WithString("namespace", mcp.Description("Namespace to delete the Pod from")),
			mcp.WithString("name", mcp.Description("Name of the Pod to delete"), mcp.Required()),
			// Tool annotations
			mcp.WithTitleAnnotation("Pods: Delete"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.podsDelete},
		{Tool: mcp.NewTool("pods_top",
			mcp.WithDescription("List the resource consumption (CPU and memory) as recorded by the Kubernetes Metrics Server for the specified Kubernetes Pods in the all namespaces, the provided namespace, or the current namespace"),
			mcp.WithBoolean("all_namespaces", mcp.Description("If true, list the resource consumption for all Pods in all namespaces. If false, list the resource consumption for Pods in the provided namespace or the current namespace"), mcp.DefaultBool(true)),
			mcp.WithString("namespace", mcp.Description("Namespace to get the Pods resource consumption from (Optional, current namespace if not provided and all_namespaces is false)")),
			mcp.WithString("name", mcp.Description("Name of the Pod to get the resource consumption from (Optional, all Pods in the namespace if not provided)")),
			mcp.WithString("label_selector", mcp.Description("Kubernetes label selector (e.g. 'app=myapp,env=prod' or 'app in (myapp,yourapp)'), use this option when you want to filter the pods by label (Optional, only applicable when name is not provided)"), mcp.Pattern("([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]")),
			// Tool annotations
			mcp.WithTitleAnnotation("Pods: Top"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.podsTop},
		{Tool: mcp.NewTool("pods_exec",
			mcp.WithDescription("Execute a command in a Kubernetes Pod in the current or provided namespace with the provided name and command"),
			mcp.WithString("namespace", mcp.Description("Namespace of the Pod where the command will be executed")),
			mcp.WithString("name", mcp.Description("Name of the Pod where the command will be executed"), mcp.Required()),
			mcp.WithArray("command", mcp.Description("Command to execute in the Pod container. "+
				"The first item is the command to be run, and the rest are the arguments to that command. "+
				`Example: ["ls", "-l", "/tmp"]`),
				// TODO: manual fix to ensure that the items property gets initialized (Gemini)
				// https://www.googlecloudcommunity.com/gc/AI-ML/Gemini-API-400-Bad-Request-Array-fields-breaks-function-calling/m-p/769835?nobounce
				func(schema map[string]interface{}) {
					schema["type"] = "array"
					schema["items"] = map[string]interface{}{
						"type": "string",
					}
				},
				mcp.Required(),
			),
			mcp.WithString("container", mcp.Description("Name of the Pod container where the command will be executed (Optional)")),
			// Tool annotations
			mcp.WithTitleAnnotation("Pods: Exec"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true), // Depending on the Pod's entrypoint, executing certain commands may kill the Pod
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.podsExec},
		{Tool: mcp.NewTool("pods_log",
			mcp.WithDescription("Get the logs of a Kubernetes Pod in the current or provided namespace with the provided name"),
			mcp.WithString("namespace", mcp.Description("Namespace to get the Pod logs from")),
			mcp.WithString("name", mcp.Description("Name of the Pod to get the logs from"), mcp.Required()),
			mcp.WithString("container", mcp.Description("Name of the Pod container to get the logs from (Optional)")),
			// Tool annotations
			mcp.WithTitleAnnotation("Pods: Log"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.podsLog},
		{Tool: mcp.NewTool("pods_run",
			mcp.WithDescription("Run a Kubernetes Pod in the current or provided namespace with the provided container image and optional name"),
			mcp.WithString("namespace", mcp.Description("Namespace to run the Pod in")),
			mcp.WithString("name", mcp.Description("Name of the Pod (Optional, random name if not provided)")),
			mcp.WithString("image", mcp.Description("Container Image to run in the Pod"), mcp.Required()),
			mcp.WithNumber("port", mcp.Description("TCP/IP port to expose from the Pod container (Optional, no port exposed if not provided)")),
			// Tool annotations
			mcp.WithTitleAnnotation("Pods: Run"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.podsRun},
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
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.PodsLog(ctx, ns.(string), name.(string), container.(string))
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
