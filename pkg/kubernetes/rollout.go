package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// revisionAnnotation is the annotation key kubectl uses to track rollout revisions.
	revisionAnnotation = "deployment.kubernetes.io/revision"
)

// DeploymentRolloutStatus returns a human-readable status of a deployment
// rollout, mirroring `kubectl rollout status deployment`.
func (c *Core) DeploymentRolloutStatus(ctx context.Context, namespace, name string) (string, error) {
	namespace = c.NamespaceOrDefault(namespace)
	dep, err := c.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}
	return deploymentStatusMessage(dep), nil
}

// deploymentStatusMessage mimics the messages printed by kubectl rollout status.
func deploymentStatusMessage(dep *appsv1.Deployment) string {
	if dep.Generation <= dep.Status.ObservedGeneration {
		cond := getDeploymentCondition(dep.Status, appsv1.DeploymentProgressing)
		if cond != nil && cond.Reason == "ProgressDeadlineExceeded" {
			return fmt.Sprintf("deployment %s has exceeded its progress deadline.", dep.Name)
		}
		if dep.Spec.Replicas != nil && dep.Status.UpdatedReplicas < *dep.Spec.Replicas {
			return fmt.Sprintf("Waiting for deployment %q rollout to finish: %d out of %d new replicas have been updated...\n",
				dep.Name, dep.Status.UpdatedReplicas, *dep.Spec.Replicas)
		}
		if dep.Status.Replicas > dep.Status.UpdatedReplicas {
			return fmt.Sprintf("Waiting for deployment %q rollout to finish: %d old replicas are pending termination...\n",
				dep.Name, dep.Status.Replicas-dep.Status.UpdatedReplicas)
		}
		if dep.Status.AvailableReplicas < dep.Status.UpdatedReplicas {
			return fmt.Sprintf("Waiting for deployment %q rollout to finish: %d of %d updated replicas are available...\n",
				dep.Name, dep.Status.AvailableReplicas, dep.Status.UpdatedReplicas)
		}
		return fmt.Sprintf("deployment %q successfully rolled out\n", dep.Name)
	}
	return fmt.Sprintf("Waiting for deployment %q rollout to finish: deployment spec is being updated...\n", dep.Name)
}

func getDeploymentCondition(status appsv1.DeploymentStatus, condType appsv1.DeploymentConditionType) *appsv1.DeploymentCondition {
	for i := range status.Conditions {
		if status.Conditions[i].Type == condType {
			return &status.Conditions[i]
		}
	}
	return nil
}

// DeploymentRolloutHistory returns the rollout history (revisions) of a
// deployment by inspecting its ReplicaSets, mirroring `kubectl rollout history`.
func (c *Core) DeploymentRolloutHistory(ctx context.Context, namespace, name string) (string, error) {
	namespace = c.NamespaceOrDefault(namespace)
	dep, err := c.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}
	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return "", fmt.Errorf("failed to parse deployment selector: %w", err)
	}
	rsList, err := c.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return "", fmt.Errorf("failed to list replicasets for deployment %s: %w", name, err)
	}

	type revInfo struct {
		revision string
		image    string
		current  bool
	}
	var revs []revInfo
	for i := range rsList.Items {
		rs := &rsList.Items[i]
		rev, ok := rs.Annotations[revisionAnnotation]
		if !ok {
			continue
		}
		img := ""
		if len(rs.Spec.Template.Spec.Containers) > 0 {
			img = rs.Spec.Template.Spec.Containers[0].Image
		}
		revs = append(revs, revInfo{
			revision: rev,
			image:    img,
			current: rs.Annotations["deployment.kubernetes.io/revision"] != "" &&
				dep.Annotations[revisionAnnotation] == rev,
		})
	}
	sort.Slice(revs, func(i, j int) bool {
		ai, _ := strconv.Atoi(revs[i].revision)
		aj, _ := strconv.Atoi(revs[j].revision)
		return ai < aj
	})

	var b strings.Builder
	b.WriteString(fmt.Sprintf("deployment/%s\n", name))
	b.WriteString("REVISION\tCHANGE-CAUSE\tIMAGE\n")
	for _, r := range revs {
		changeCause := ""
		if r.current {
			changeCause = "<current>"
		}
		b.WriteString(fmt.Sprintf("%s\t%s\t%s\n", r.revision, changeCause, r.image))
	}
	if len(revs) == 0 {
		b.WriteString("No rollout history found\n")
	}
	return b.String(), nil
}

// DeploymentRolloutUndo rolls a deployment back to a previous revision. If
// toRevision is 0 it rolls back to the previous revision. Mirrors
// `kubectl rollout undo`.
func (c *Core) DeploymentRolloutUndo(ctx context.Context, namespace, name string, toRevision int64) (string, error) {
	namespace = c.NamespaceOrDefault(namespace)
	dep, err := c.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}
	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return "", fmt.Errorf("failed to parse deployment selector: %w", err)
	}
	rsList, err := c.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return "", fmt.Errorf("failed to list replicasets for deployment %s: %w", name, err)
	}

	currentRev := dep.Annotations[revisionAnnotation]
	var target *appsv1.ReplicaSet
	if toRevision == 0 {
		// Find the previous (second-newest) revision.
		var all []appsv1.ReplicaSet
		for i := range rsList.Items {
			rs := &rsList.Items[i]
			if _, ok := rs.Annotations[revisionAnnotation]; !ok {
				continue
			}
			if rs.Annotations[revisionAnnotation] == currentRev {
				continue
			}
			all = append(all, *rs)
		}
		sort.Slice(all, func(i, j int) bool {
			ai, _ := strconv.Atoi(all[i].Annotations[revisionAnnotation])
			aj, _ := strconv.Atoi(all[j].Annotations[revisionAnnotation])
			return ai > aj
		})
		if len(all) == 0 {
			return "", fmt.Errorf("no previous revision found to roll back to for deployment %s", name)
		}
		target = &all[0]
	} else {
		for i := range rsList.Items {
			rs := &rsList.Items[i]
			if rs.Annotations[revisionAnnotation] == strconv.FormatInt(toRevision, 10) {
				target = rs
				break
			}
		}
		if target == nil {
			return "", fmt.Errorf("revision %d not found in rollout history of deployment %s", toRevision, name)
		}
	}

	// Patch the deployment spec template with the target ReplicaSet's template.
	patchObj := map[string]interface{}{
		"spec": map[string]interface{}{
			"template": target.Spec.Template,
		},
	}
	patch, err := json.Marshal(patchObj)
	if err != nil {
		return "", fmt.Errorf("failed to encode rollback patch: %w", err)
	}
	// Apply the template patch using a strategic merge patch.
	_, err = c.AppsV1().Deployments(namespace).Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to patch deployment %s for rollback: %w", name, err)
	}
	verb := "previous revision"
	if toRevision != 0 {
		verb = fmt.Sprintf("revision %d", toRevision)
	}
	return fmt.Sprintf("deployment/%s rolled back to %s", name, verb), nil
}

// DeploymentRestart triggers a rolling restart of a deployment by patching the
// pod template annotation with a timestamp, mirroring `kubectl rollout restart`.
func (c *Core) DeploymentRestart(ctx context.Context, namespace, name string) (string, error) {
	namespace = c.NamespaceOrDefault(namespace)
	dep, err := c.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = map[string]string{}
	}
	dep.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] =
		metav1.Now().Format("2006-01-02T15:04:05Z07:00")

	_, err = c.AppsV1().Deployments(namespace).Update(ctx, dep, metav1.UpdateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to restart deployment %s: %w", name, err)
	}
	return fmt.Sprintf("deployment/%s restarted", name), nil
}
