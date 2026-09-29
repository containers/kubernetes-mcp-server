package kiali

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
func internalServiceURLs(cr *KialiCR) []string {
	if cr == nil {
		return nil
	}
	instanceName := cr.Status.Deployment.InstanceName
	if instanceName == "" {
		instanceName = cr.Spec.Deployment.InstanceName
	}
	if instanceName == "" {
		instanceName = cr.GetName()
	}
	if instanceName == "" {
		instanceName = "kiali"
	}

	ns := cr.Status.Deployment.Namespace
	if ns == "" {
		ns = cr.Spec.Deployment.Namespace
	}
	if ns == "" {
		ns = cr.GetNamespace()
	}
	if ns == "" {
		return nil
	}

	port := cr.Spec.Server.Port
	if port <= 0 {
		port = defaultKialiPort
	}

	webRoot := cr.Spec.Server.WebRoot
	base := fmt.Sprintf("http://%s.%s.svc:%d", instanceName, ns, port)

	switch webRoot {
	case "", "/":
		// Operator defaults: "/" on OpenShift, "/kiali" elsewhere. Try both.
		return []string{base, base + "/kiali"}
	default:
		return []string{base + strings.TrimSuffix(webRoot, "/")}
	}
}

func kialiFromUnstructured(obj *unstructured.Unstructured) (*KialiCR, error) {
	if obj == nil {
		return nil, fmt.Errorf("kiali CR is nil")
	}
	var cr KialiCR
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &cr); err != nil {
		return nil, err
	}
	return &cr, nil
}

// listKialiCRs returns all Kiali CRs across namespaces. Returns nil when none exist or on error.
func listKialiCRs(ctx context.Context, dc dynamic.Interface) []*KialiCR {
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
	out := make([]*KialiCR, 0, len(list.Items))
	for i := range list.Items {
		cr, err := kialiFromUnstructured(&list.Items[i])
		if err != nil {
			klogutil.FromContext(ctx).V(2).Info("failed to decode Kiali CR", "error", err)
			continue
		}
		out = append(out, cr)
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

	candidates := make([]string, 0, len(crs)*2)
	for _, cr := range crs {
		candidates = append(candidates, internalServiceURLs(cr)...)
	}
	url, ok := probeCandidateURLs(ctx, candidates, cfg, bearerToken)
	if ok {
		klogutil.FromContext(ctx).V(1).Info("discovered reachable Kiali URL from CR", "url", url)
		return url, true
	}
	klogutil.FromContext(ctx).V(1).Info("Kiali CR(s) found but no reachable in-cluster URL")
	return "", false
}
