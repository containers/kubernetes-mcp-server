package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubectl/pkg/drain"
)

// NodeDrainOptions controls node drain behaviour, mirroring kubectl drain flags.
type NodeDrainOptions struct {
	Force               bool
	IgnoreAllDaemonSets bool
	DeleteEmptyDirData  bool
	DisableEviction     bool
	GracePeriodSeconds  int
	Timeout             int
}

// NodeCordon marks a node as unschedulable, mirroring `kubectl cordon`.
func (c *Core) NodeCordon(ctx context.Context, name string) (string, error) {
	return c.nodeSetUnschedulable(ctx, name, true)
}

// NodeUncordon marks a node as schedulable, mirroring `kubectl uncordon`.
func (c *Core) NodeUncordon(ctx context.Context, name string) (string, error) {
	return c.nodeSetUnschedulable(ctx, name, false)
}

func (c *Core) nodeSetUnschedulable(ctx context.Context, name string, unschedulable bool) (string, error) {
	if _, err := c.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{}); err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", name, err)
	}
	patch := fmt.Sprintf(`{"spec":{"unschedulable":%t}}`, unschedulable)
	if _, err := c.CoreV1().Nodes().Patch(ctx, name, types.MergePatchType, []byte(patch), metav1.PatchOptions{}); err != nil {
		return "", fmt.Errorf("failed to patch node %s: %w", name, err)
	}
	verb := "cordoned"
	if !unschedulable {
		verb = "uncordoned"
	}
	return fmt.Sprintf("node/%s %s", name, verb), nil
}

// NodeDrain evicts all pods from a node, mirroring `kubectl drain`.
// It cordons the node first, then evicts pods honoring the provided options.
func (c *Core) NodeDrain(ctx context.Context, name string, opts NodeDrainOptions) (string, error) {
	node, err := c.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", name, err)
	}

	// Cordon first so no new pods land while we drain.
	if _, err := c.NodeCordon(ctx, name); err != nil {
		return "", fmt.Errorf("failed to cordon node %s before draining: %w", name, err)
	}

	helper := &drain.Helper{
		Ctx:                 ctx,
		Client:              c,
		Force:               opts.Force,
		GracePeriodSeconds:  opts.GracePeriodSeconds,
		IgnoreAllDaemonSets: opts.IgnoreAllDaemonSets,
		Timeout:             time.Duration(opts.Timeout) * time.Second,
		DeleteEmptyDirData:  opts.DeleteEmptyDirData,
		DisableEviction:     opts.DisableEviction,
		Out:                 io.Discard,
		ErrOut:              os.Stderr,
	}

	if err := drain.RunCordonOrUncordon(helper, node, true); err != nil {
		return "", fmt.Errorf("failed to cordon during drain: %w", err)
	}
	if err := drain.RunNodeDrain(helper, name); err != nil {
		return "", fmt.Errorf("failed to drain node %s: %w", name, err)
	}
	return fmt.Sprintf("node/%s drained", name), nil
}

// NodePatchLabel sets or removes a single label on a node. A value of ""
// removes the label.
func (c *Core) NodePatchLabel(ctx context.Context, name, key, value string) (string, error) {
	if key == "" {
		return "", errors.New("label key must not be empty")
	}
	if value == "" {
		patch := fmt.Sprintf(`[{"op":"remove","path":"/metadata/labels/%s"}]`, escapeJSONPointer(key))
		if _, err := c.CoreV1().Nodes().Patch(ctx, name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{}); err != nil {
			return "", fmt.Errorf("failed to remove label %s on node %s: %w", key, name, err)
		}
		return fmt.Sprintf("label %s removed from node/%s", key, name), nil
	}
	patch := fmt.Sprintf(`{"metadata":{"labels":{%q:%q}}}`, key, value)
	if _, err := c.CoreV1().Nodes().Patch(ctx, name, types.MergePatchType, []byte(patch), metav1.PatchOptions{}); err != nil {
		return "", fmt.Errorf("failed to set label %s on node %s: %w", key, name, err)
	}
	return fmt.Sprintf("label %s=%s set on node/%s", key, value, name), nil
}

// NodePatchTaint sets or removes a single taint on a node.
func (c *Core) NodePatchTaint(ctx context.Context, name, key, value, effect string, remove bool) (string, error) {
	if key == "" || effect == "" {
		return "", errors.New("taint key and effect must not be empty")
	}
	var patch []byte
	if remove {
		patch = []byte(fmt.Sprintf(`{"spec":{"taints":[{"key":%q,"effect":%q,"$patch":"delete"}]}}`, key, effect))
	} else {
		patch = []byte(fmt.Sprintf(`{"spec":{"taints":[{"key":%q,"value":%q,"effect":%q}]}}`, key, value, effect))
	}
	if _, err := c.CoreV1().Nodes().Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return "", fmt.Errorf("failed to patch taint on node %s: %w", name, err)
	}
	verb := "added"
	if remove {
		verb = "removed"
	}
	return fmt.Sprintf("taint %s=%s:%s %s on node/%s", key, value, effect, verb, name), nil
}

// escapeJSONPointer escapes a key per RFC 6901 for use in a JSON patch path.
func escapeJSONPointer(key string) string {
	out := make([]byte, 0, len(key))
	for i := 0; i < len(key); i++ {
		switch key[i] {
		case '~':
			out = append(out, '~', '0')
		case '/':
			out = append(out, '~', '1')
		default:
			out = append(out, key[i])
		}
	}
	return string(out)
}

// _ keeps corev1 referenced for the NodeDrain node helper.
var _ = corev1.Node{}
