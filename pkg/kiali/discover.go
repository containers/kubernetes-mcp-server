package kiali

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
)

// KialiGVR is the GroupVersionResource for Kiali CRs.
var KialiGVR = schema.GroupVersionResource{
	Group:    KialiGVK.Group,
	Version:  KialiGVK.Version,
	Resource: "kialis",
}

const defaultKialiPort int64 = 20001

// serviceURLSchemes returns scheme(s) for in-cluster Service URL candidates.
// With requireTLS, only https is used (plain http is not allowed). Otherwise both
// https and http are listed; probeCandidateURLs tries https first and only falls
// back to http if no https candidate answers GET /api/status.
func serviceURLSchemes(requireTLS bool) []string {
	if requireTLS {
		return []string{"https"}
	}
	return []string{"https", "http"}
}

// internalServiceURLs builds candidate in-cluster Service base URLs for a Kiali CR.
// Prefer values from status/spec; fall back to CR metadata and Kiali defaults.
func internalServiceURLs(cr *KialiCR, requireTLS bool) []string {
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
	// Reject host-injection via instance_name / namespace (e.g. "x@evil#", "evil.com:80#").
	if ns == "" || len(validation.IsDNS1123Label(instanceName)) > 0 || len(validation.IsDNS1123Label(ns)) > 0 {
		return nil
	}

	port := cr.Spec.Server.Port
	if port <= 0 {
		port = defaultKialiPort
	}

	schemes := serviceURLSchemes(requireTLS)

	webRoot := cr.Spec.Server.WebRoot
	out := make([]string, 0, len(schemes)*2)
	for _, scheme := range schemes {
		base := fmt.Sprintf("%s://%s.%s.svc:%d", scheme, instanceName, ns, port)
		switch webRoot {
		case "", "/":
			// Operator defaults: "/" on OpenShift, "/kiali" elsewhere. Try both.
			out = append(out, base, base+"/kiali")
		default:
			// Reject absolute/scheme-relative roots that could rewrite the authority
			// (e.g. "@evil/", "//evil/").
			if !strings.HasPrefix(webRoot, "/") || strings.HasPrefix(webRoot, "//") {
				return nil
			}
			out = append(out, base+strings.TrimSuffix(webRoot, "/"))
		}
	}
	return out
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

// listKialiCRs returns Kiali CRs in the given namespaces. Returns nil when none exist or on error.
func listKialiCRs(ctx context.Context, inspector api.ClusterInspector, namespaces []string) []*KialiCR {
	if inspector == nil || len(namespaces) == 0 {
		return nil
	}
	unstructuredAPI := inspector.Unstructured()
	if unstructuredAPI == nil {
		return nil
	}
	out := make([]*KialiCR, 0)
	for _, ns := range namespaces {
		list, err := unstructuredAPI.Resource(KialiGVR).Namespace(ns).List(ctx, metav1.ListOptions{}).Default()
		if err != nil {
			klogutil.FromContext(ctx).V(2).Info("failed to list Kiali CRs", "namespace", ns, "error", err)
			continue
		}
		if list == nil || len(list.Items) == 0 {
			continue
		}
		for i := range list.Items {
			cr, err := kialiFromUnstructured(&list.Items[i])
			if err != nil {
				klogutil.FromContext(ctx).V(2).Info("failed to decode Kiali CR", "error", err)
				continue
			}
			out = append(out, cr)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// discoverAndValidateInternalURL finds a Kiali CR, builds in-cluster Service URL candidates,
// and returns the first base URL that responds successfully to GET /api/status.
func discoverAndValidateInternalURL(ctx context.Context, inspector api.ClusterInspector, kialiCfg *Config, serverCfg *config.Config, bearerToken string) (string, bool) {
	namespaces := urlDiscoveryNamespaces(kialiCfg)
	crs := listKialiCRs(ctx, inspector, namespaces)
	if len(crs) == 0 {
		return "", false
	}

	requireTLS := serverCfg != nil && serverCfg.RequireTLS.Get()
	candidates := make([]string, 0, len(crs)*2)
	for _, cr := range crs {
		candidates = append(candidates, internalServiceURLs(cr, requireTLS)...)
	}
	url, ok := probeCandidateURLs(ctx, candidates, kialiCfg, serverCfg, bearerToken)
	if ok {
		klogutil.FromContext(ctx).V(1).Info("discovered reachable Kiali URL from CR", "url", url)
		return url, true
	}
	klogutil.FromContext(ctx).V(1).Info("Kiali CR(s) found but no reachable in-cluster URL")
	return "", false
}
