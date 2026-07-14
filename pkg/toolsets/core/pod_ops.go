package core

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

func initPodOps() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "pods_port_forward",
				Description: "Start port-forwarding a local port to a port on a Kubernetes Pod. Runs in the background and returns a session id plus the local address you can connect to. Use pods_port_forward_stop with the returned id to terminate the session. Mirrors `kubectl port-forward pod`",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace of the Pod (Optional, current namespace if not provided)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the Pod to forward to",
						},
						"local_port": {
							Type:        "integer",
							Description: "Local port to listen on. If 0 or omitted, a free port is chosen automatically",
							Minimum:     ptr.To(float64(0)),
						},
						"remote_port": {
							Type:        "integer",
							Description: "Port on the Pod to forward to (the container port)",
							Minimum:     ptr.To(float64(1)),
						},
					},
					Required: []string{"name", "remote_port"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Pods: Port Forward",
					DestructiveHint: ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: podsPortForward,
		},
		{
			Tool: api.Tool{
				Name:        "pods_port_forward_stop",
				Description: "Stop a running port-forward session by its session id, releasing the local port. Returns the session details that were stopped",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"id": {
							Type:        "string",
							Description: "Session id returned by pods_port_forward",
						},
					},
					Required: []string{"id"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Pods: Port Forward Stop",
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: podsPortForwardStop,
		},
		{
			Tool: api.Tool{
				Name:        "pods_port_forward_list",
				Description: "List all active port-forward sessions, showing their session id, target pod, namespace, and local/remote ports",
				InputSchema: &jsonschema.Schema{
					Type:       "object",
					Properties: map[string]*jsonschema.Schema{},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Pods: Port Forward List",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: podsPortForwardList,
		},
		{
			Tool: api.Tool{
				Name:        "pods_attach",
				Description: "Attach to a running container in a Kubernetes Pod, capturing its stdout and stderr. Unlike exec, attach connects to the container's primary process (PID 1) rather than starting a new command. Mirrors `kubectl attach`",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace of the Pod (Optional, current namespace if not provided)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the Pod to attach to",
						},
						"container": {
							Type:        "string",
							Description: "Name of the Pod container to attach to (Optional, defaults to the first or kubectl default-container annotation)",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Pods: Attach",
					DestructiveHint: ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: podsAttach,
		},
	}
}

func podsPortForward(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	name := p.RequiredString("name")
	localPort := int(p.OptionalInt64("local_port", 0))
	remotePort := int(p.RequiredInt64("remote_port"))
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to start port forward: %w", err)), nil
	}
	session, err := kubernetes.NewCore(params).PodsPortForward(params.Context, ns, name, localPort, remotePort)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to start port forward to pod %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(fmt.Sprintf(
		"Port forward started (session %s): 127.0.0.1:%d -> %s/%s:%d\nUse pods_port_forward_stop with id %s to stop.",
		session.ID, session.LocalPort, session.Namespace, session.PodName, session.RemotePort, session.ID,
	), nil), nil
}

func podsPortForwardStop(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	id := p.RequiredString("id")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to stop port forward: %w", err)), nil
	}
	if err := kubernetes.StopPortForward(id); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to stop port forward %s: %w", id, err)), nil
	}
	return api.NewToolCallResult(fmt.Sprintf("Port forward session %s stopped", id), nil), nil
}

func podsPortForwardList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	sessions := kubernetes.ListPortForwards()
	if len(sessions) == 0 {
		return api.NewToolCallResult("No active port forward sessions", nil), nil
	}
	var b []byte
	b = fmtAppendf(b, "ID\tNAMESPACE\tPOD\tLOCAL\tREMOTE\n")
	for _, s := range sessions {
		b = fmtAppendf(b, "%s\t%s\t%s\t%d\t%d\n", s.ID, s.Namespace, s.PodName, s.LocalPort, s.RemotePort)
	}
	return api.NewToolCallResult(string(b), nil), nil
}

func podsAttach(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	name := p.RequiredString("name")
	container := p.OptionalString("container", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to attach to pod: %w", err)), nil
	}
	stdout, stderr, err := kubernetes.NewCore(params).PodsAttach(params.Context, ns, name, container)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to attach to pod %s in namespace %s: %w", name, ns, err)), nil
	}
	ret := stdout
	if ret == "" && stderr != "" {
		ret = stderr
	}
	if ret == "" {
		ret = fmt.Sprintf("Attached to pod %s in namespace %s; no output produced", name, ns)
	}
	return api.NewToolCallResult(ret, nil), nil
}

// fmtAppendf appends a formatted string to a byte slice.
func fmtAppendf(b []byte, format string, args ...interface{}) []byte {
	return append(b, []byte(fmt.Sprintf(format, args...))...)
}
