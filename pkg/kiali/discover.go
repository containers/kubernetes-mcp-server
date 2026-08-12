package kiali

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
)

// KialiGVR is the GroupVersionResource for Kiali CRs.
var KialiGVR = schema.GroupVersionResource{
	Group:    KialiGVK.Group,
	Version:  KialiGVK.Version,
	Resource: "kialis",
}

const defaultKialiPort int64 = 20001

// internalServiceURLs builds candidate in-cluster Service base URLs for a Kiali CR.
// Prefer values from status/spec; fall back to CR metadata and Kiali defaults.
func internalServiceURLs(cr *unstructured.Unstructured) []string {
	if cr == nil {
		return nil
	}
	instanceName := nestedString(cr.Object, "status", "deployment", "instanceName")
	if instanceName == "" {
		instanceName = nestedString(cr.Object, "spec", "deployment", "instance_name")
	}
	if instanceName == "" {
		instanceName = cr.GetName()
	}
	if instanceName == "" {
		instanceName = "kiali"
	}

	ns := nestedString(cr.Object, "status", "deployment", "namespace")
	if ns == "" {
		ns = nestedString(cr.Object, "spec", "deployment", "namespace")
	}
	if ns == "" {
		ns = cr.GetNamespace()
	}
	if ns == "" {
		return nil
	}

	port := nestedInt64(cr.Object, "spec", "server", "port")
	if port <= 0 {
		port = defaultKialiPort
	}

	webRoot := nestedString(cr.Object, "spec", "server", "web_root")
	base := fmt.Sprintf("http://%s.%s.svc:%d", instanceName, ns, port)

	switch webRoot {
	case "", "/":
		// Operator defaults: "/" on OpenShift, "/kiali" elsewhere. Try both.
		return []string{base, base + "/kiali"}
	default:
		return []string{base + strings.TrimSuffix(webRoot, "/")}
	}
}

func nestedString(obj map[string]any, fields ...string) string {
	v, found, err := unstructured.NestedString(obj, fields...)
	if !found || err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

func nestedInt64(obj map[string]any, fields ...string) int64 {
	v, found, err := unstructured.NestedInt64(obj, fields...)
	if !found || err != nil {
		return 0
	}
	return v
}

// listKialiCRs returns all Kiali CRs across namespaces. Returns nil when none exist or on error.
func listKialiCRs(ctx context.Context, dc dynamic.Interface) []*unstructured.Unstructured {
	if dc == nil {
		return nil
	}
	list, err := dc.Resource(KialiGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		klogutil.FromContext(ctx).V(2).Info("failed to list Kiali CRs", "error", err)
		return nil
	}
	if list == nil || len(list.Items) == 0 {
		return nil
	}
	out := make([]*unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, &list.Items[i])
	}
	return out
}

// discoverAndValidateInternalURL finds a Kiali CR, builds in-cluster Service URL candidates,
// and returns the first base URL that responds successfully to GET /api/status.
func discoverAndValidateInternalURL(ctx context.Context, dc dynamic.Interface, cfg *Config, bearerToken string) (string, bool) {
	crs := listKialiCRs(ctx, dc)
	if len(crs) == 0 {
		return "", false
	}
	for _, cr := range crs {
		for _, candidate := range internalServiceURLs(cr) {
			if probeStatusURL(ctx, candidate, cfg, bearerToken) {
				klogutil.FromContext(ctx).V(1).Info("discovered reachable Kiali URL from CR",
					"url", candidate, "kiali_cr", cr.GetNamespace()+"/"+cr.GetName())
				return candidate, true
			}
		}
	}
	klogutil.FromContext(ctx).V(1).Info("Kiali CR(s) found but no reachable in-cluster URL")
	return "", false
}
