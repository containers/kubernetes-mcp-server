package kubernetes

import (
	"context"
	"fmt"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

// --- HorizontalPodAutoscaler ---

// HPAsList lists HorizontalPodAutoscalers in the given namespace (all namespaces
// if namespace is empty).
func (c *Core) HPAsList(ctx context.Context, namespace string) (string, error) {
	list, err := c.AutoscalingV2().HorizontalPodAutoscalers(c.NamespaceOrDefault(namespace)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list horizontalpodautoscalers: %w", err)
	}
	return output.MarshalYaml(list)
}

// HPAGet returns a single HorizontalPodAutoscaler.
func (c *Core) HPAGet(ctx context.Context, namespace, name string) (string, error) {
	hpa, err := c.AutoscalingV2().HorizontalPodAutoscalers(c.NamespaceOrDefault(namespace)).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get horizontalpodautoscaler %s: %w", name, err)
	}
	return output.MarshalYaml(hpa)
}

// HPACreate creates a HorizontalPodAutoscaler from a YAML/JSON manifest string.
func (c *Core) HPACreate(ctx context.Context, manifest string) (string, error) {
	obj, err := decodeManifest(manifest)
	if err != nil {
		return "", err
	}
	hpa, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok {
		return "", fmt.Errorf("manifest is not a HorizontalPodAutoscaler")
	}
	created, err := c.AutoscalingV2().HorizontalPodAutoscalers(c.NamespaceOrDefault(hpa.Namespace)).Create(ctx, hpa, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create horizontalpodautoscaler: %w", err)
	}
	return output.MarshalYaml(created)
}

// HPADelete deletes a HorizontalPodAutoscaler.
func (c *Core) HPADelete(ctx context.Context, namespace, name string) (string, error) {
	if err := c.AutoscalingV2().HorizontalPodAutoscalers(c.NamespaceOrDefault(namespace)).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return "", fmt.Errorf("failed to delete horizontalpodautoscaler %s: %w", name, err)
	}
	return fmt.Sprintf("horizontalpodautoscaler/%s deleted", name), nil
}

// --- PodDisruptionBudget ---

// PDBsList lists PodDisruptionBudgets in the given namespace (all namespaces if
// namespace is empty).
func (c *Core) PDBsList(ctx context.Context, namespace string) (string, error) {
	list, err := c.PolicyV1().PodDisruptionBudgets(c.NamespaceOrDefault(namespace)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list poddisruptionbudgets: %w", err)
	}
	return output.MarshalYaml(list)
}

// PDBGet returns a single PodDisruptionBudget.
func (c *Core) PDBGet(ctx context.Context, namespace, name string) (string, error) {
	pdb, err := c.PolicyV1().PodDisruptionBudgets(c.NamespaceOrDefault(namespace)).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get poddisruptionbudget %s: %w", name, err)
	}
	return output.MarshalYaml(pdb)
}

// PDBCreate creates a PodDisruptionBudget from a YAML/JSON manifest string.
func (c *Core) PDBCreate(ctx context.Context, manifest string) (string, error) {
	obj, err := decodeManifest(manifest)
	if err != nil {
		return "", err
	}
	pdb, ok := obj.(*policyv1.PodDisruptionBudget)
	if !ok {
		return "", fmt.Errorf("manifest is not a PodDisruptionBudget")
	}
	created, err := c.PolicyV1().PodDisruptionBudgets(c.NamespaceOrDefault(pdb.Namespace)).Create(ctx, pdb, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create poddisruptionbudget: %w", err)
	}
	return output.MarshalYaml(created)
}

// PDBDelete deletes a PodDisruptionBudget.
func (c *Core) PDBDelete(ctx context.Context, namespace, name string) (string, error) {
	if err := c.PolicyV1().PodDisruptionBudgets(c.NamespaceOrDefault(namespace)).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return "", fmt.Errorf("failed to delete poddisruptionbudget %s: %w", name, err)
	}
	return fmt.Sprintf("poddisruptionbudget/%s deleted", name), nil
}

// --- ResourceQuota ---

// ResourceQuotasList lists ResourceQuotas in the given namespace (all
// namespaces if empty).
func (c *Core) ResourceQuotasList(ctx context.Context, namespace string) (string, error) {
	list, err := c.CoreV1().ResourceQuotas(c.NamespaceOrDefault(namespace)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list resourcequotas: %w", err)
	}
	return output.MarshalYaml(list)
}

// ResourceQuotaGet returns a single ResourceQuota.
func (c *Core) ResourceQuotaGet(ctx context.Context, namespace, name string) (string, error) {
	rq, err := c.CoreV1().ResourceQuotas(c.NamespaceOrDefault(namespace)).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get resourcequota %s: %w", name, err)
	}
	return output.MarshalYaml(rq)
}

// ResourceQuotaCreate creates a ResourceQuota from a YAML/JSON manifest string.
func (c *Core) ResourceQuotaCreate(ctx context.Context, manifest string) (string, error) {
	obj, err := decodeManifest(manifest)
	if err != nil {
		return "", err
	}
	rq, ok := obj.(*corev1.ResourceQuota)
	if !ok {
		return "", fmt.Errorf("manifest is not a ResourceQuota")
	}
	created, err := c.CoreV1().ResourceQuotas(c.NamespaceOrDefault(rq.Namespace)).Create(ctx, rq, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create resourcequota: %w", err)
	}
	return output.MarshalYaml(created)
}

// ResourceQuotaDelete deletes a ResourceQuota.
func (c *Core) ResourceQuotaDelete(ctx context.Context, namespace, name string) (string, error) {
	if err := c.CoreV1().ResourceQuotas(c.NamespaceOrDefault(namespace)).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return "", fmt.Errorf("failed to delete resourcequota %s: %w", name, err)
	}
	return fmt.Sprintf("resourcequota/%s deleted", name), nil
}

// --- LimitRange ---

// LimitRangesList lists LimitRanges in the given namespace (all namespaces if
// empty).
func (c *Core) LimitRangesList(ctx context.Context, namespace string) (string, error) {
	list, err := c.CoreV1().LimitRanges(c.NamespaceOrDefault(namespace)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list limitranges: %w", err)
	}
	return output.MarshalYaml(list)
}

// LimitRangeGet returns a single LimitRange.
func (c *Core) LimitRangeGet(ctx context.Context, namespace, name string) (string, error) {
	lr, err := c.CoreV1().LimitRanges(c.NamespaceOrDefault(namespace)).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get limitrange %s: %w", name, err)
	}
	return output.MarshalYaml(lr)
}

// LimitRangeCreate creates a LimitRange from a YAML/JSON manifest string.
func (c *Core) LimitRangeCreate(ctx context.Context, manifest string) (string, error) {
	obj, err := decodeManifest(manifest)
	if err != nil {
		return "", err
	}
	lr, ok := obj.(*corev1.LimitRange)
	if !ok {
		return "", fmt.Errorf("manifest is not a LimitRange")
	}
	created, err := c.CoreV1().LimitRanges(c.NamespaceOrDefault(lr.Namespace)).Create(ctx, lr, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create limitrange: %w", err)
	}
	return output.MarshalYaml(created)
}

// LimitRangeDelete deletes a LimitRange.
func (c *Core) LimitRangeDelete(ctx context.Context, namespace, name string) (string, error) {
	if err := c.CoreV1().LimitRanges(c.NamespaceOrDefault(namespace)).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return "", fmt.Errorf("failed to delete limitrange %s: %w", name, err)
	}
	return fmt.Sprintf("limitrange/%s deleted", name), nil
}
