package olm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// Kubernetes exposes a small subset of the manager methods used by the OLM wrapper
type Kubernetes interface {
	ToRESTConfig() (*rest.Config, error)
	ToRESTMapper() (meta.RESTMapper, error)
	ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error)
	NamespaceOrDefault(namespace string) string
}

type Olm struct {
	kubernetes Kubernetes
}

func NewOlm(k Kubernetes) *Olm {
	return &Olm{kubernetes: k}
}

func parseManifest(manifest string) (*unstructured.Unstructured, error) {
	// Try YAML -> JSON -> map
	var obj map[string]interface{}
	jsonBytes, err := yaml.YAMLToJSON([]byte(manifest))
	if err != nil {
		// try raw JSON
		if err := json.Unmarshal([]byte(manifest), &obj); err != nil {
			return nil, fmt.Errorf("failed to decode manifest: %w", err)
		}
	} else {
		if err := json.Unmarshal(jsonBytes, &obj); err != nil {
			return nil, fmt.Errorf("failed to decode manifest JSON: %w", err)
		}
	}
	return &unstructured.Unstructured{Object: obj}, nil
}

// findGVR attempts to resolve a GroupVersionResource for the provided unstructured object.
// It first tries the RESTMapper (preferred), then falls back to scanning discovery resources.
func (o *Olm) findGVR(u *unstructured.Unstructured) (schema.GroupVersionResource, error) {
	// Try RESTMapper first
	if mapper, err := o.kubernetes.ToRESTMapper(); err == nil {
		gvk := u.GroupVersionKind()
		if gvk.Empty() {
			gvk = schema.FromAPIVersionAndKind(u.GetAPIVersion(), u.GetKind())
		}
		if !gvk.Empty() {
			if mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version); err == nil {
				return mapping.Resource, nil
			}
		}
	}

	// Fallback: scan discovery for a resource with the same Kind
	disc, err := o.kubernetes.ToDiscoveryClient()
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("failed to get discovery client: %w", err)
	}
	lists, _ := disc.ServerPreferredResources()
	for _, apiList := range lists {
		gv, _ := schema.ParseGroupVersion(apiList.GroupVersion)
		for _, r := range apiList.APIResources {
			if strings.EqualFold(r.Kind, u.GetKind()) || r.Name == strings.ToLower(u.GetKind()) || r.Name == strings.ToLower(u.GetKind())+"s" {
				return gv.WithResource(r.Name), nil
			}
		}
	}
	return schema.GroupVersionResource{}, fmt.Errorf("resource for kind '%s' not found", u.GetKind())
}

// Install creates or updates a ClusterExtension (or other OLMv1-managed) resource from a manifest (YAML/JSON)
func (o *Olm) Install(ctx context.Context, manifest string) (string, error) {
	cfg, err := o.kubernetes.ToRESTConfig()
	if err != nil {
		return "", err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return "", err
	}
	u, err := parseManifest(manifest)
	if err != nil {
		return "", err
	}
	if u.GetName() == "" {
		return "", fmt.Errorf("manifest must include metadata.name")
	}
	gvr, err := o.findGVR(u)
	if err != nil {
		return "", err
	}

	// Determine if the resource is namespaced using the RESTMapper if possible
	namespaced := false
	if mapper, err := o.kubernetes.ToRESTMapper(); err == nil {
		gvk := u.GroupVersionKind()
		if gvk.Empty() {
			gvk = schema.FromAPIVersionAndKind(u.GetAPIVersion(), u.GetKind())
		}
		if !gvk.Empty() {
			if mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version); err == nil {
				namespaced = mapping.Scope.Name() == meta.RESTScopeNameNamespace
			}
		}
	}

	var created *unstructured.Unstructured
	if namespaced {
		ns := u.GetNamespace()
		if ns == "" {
			ns = o.kubernetes.NamespaceOrDefault("")
		}
		created, err = dyn.Resource(gvr).Namespace(ns).Create(ctx, u, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			existing, getErr := dyn.Resource(gvr).Namespace(ns).Get(ctx, u.GetName(), metav1.GetOptions{})
			if getErr != nil {
				return "", getErr
			}
			u.SetResourceVersion(existing.GetResourceVersion())
			created, err = dyn.Resource(gvr).Namespace(ns).Update(ctx, u, metav1.UpdateOptions{})
		}
	} else {
		created, err = dyn.Resource(gvr).Create(ctx, u, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			existing, getErr := dyn.Resource(gvr).Get(ctx, u.GetName(), metav1.GetOptions{})
			if getErr != nil {
				return "", getErr
			}
			u.SetResourceVersion(existing.GetResourceVersion())
			created, err = dyn.Resource(gvr).Update(ctx, u, metav1.UpdateOptions{})
		}
	}
	if err != nil {
		return "", err
	}
	out, err := yaml.Marshal(created.Object)
	if err != nil {
		// fallback to JSON string
		b, _ := json.Marshal(created.Object)
		return string(b), nil
	}
	return string(out), nil
}

// List lists ClusterExtension resources (or other discovered kinds with the ClusterExtension kind)
func (o *Olm) List(ctx context.Context) (string, error) {
	cfg, err := o.kubernetes.ToRESTConfig()
	if err != nil {
		return "", err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return "", err
	}
	// Discover the GVR for ClusterExtension
	disc, err := o.kubernetes.ToDiscoveryClient()
	if err != nil {
		return "", err
	}
	lists, err := disc.ServerPreferredResources()
	if err != nil {
		// continue if partial success
	}
	var gvr schema.GroupVersionResource
	found := false
	for _, apiList := range lists {
		gv, _ := schema.ParseGroupVersion(apiList.GroupVersion)
		for _, r := range apiList.APIResources {
			if strings.EqualFold(r.Kind, "ClusterExtension") || r.Name == "clusterextensions" {
				gvr = gv.WithResource(r.Name)
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return "", fmt.Errorf("ClusterExtension resource not found on the cluster")
	}
	list, err := dyn.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	// Convert items to a slice of plain maps for nicer YAML output
	items := make([]map[string]interface{}, 0, len(list.Items))
	for _, it := range list.Items {
		items = append(items, it.Object)
	}
	out, err := yaml.Marshal(items)
	if err != nil {
		b, _ := json.Marshal(items)
		return string(b), nil
	}
	return string(out), nil
}

// Uninstall deletes a ClusterExtension by name (cluster-scoped)
func (o *Olm) Uninstall(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	cfg, err := o.kubernetes.ToRESTConfig()
	if err != nil {
		return "", err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return "", err
	}
	// Find the ClusterExtension GVR
	disc, err := o.kubernetes.ToDiscoveryClient()
	if err != nil {
		return "", err
	}
	lists, err := disc.ServerPreferredResources()
	if err != nil {
		// continue if partial
	}
	var gvr schema.GroupVersionResource
	found := false
	for _, apiList := range lists {
		gv, _ := schema.ParseGroupVersion(apiList.GroupVersion)
		for _, r := range apiList.APIResources {
			if strings.EqualFold(r.Kind, "ClusterExtension") || r.Name == "clusterextensions" {
				gvr = gv.WithResource(r.Name)
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return "", fmt.Errorf("ClusterExtension resource not found on the cluster")
	}
	err = dyn.Resource(gvr).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return fmt.Sprintf("ClusterExtension %s not found", name), nil
	} else if err != nil {
		return "", err
	}
	return fmt.Sprintf("ClusterExtension %s deleted", name), nil
}

// Upgrade updates an existing ClusterExtension resource by name using the provided manifest.
// It fails if the named resource does not exist.
func (o *Olm) Upgrade(ctx context.Context, name string, manifest string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	cfg, err := o.kubernetes.ToRESTConfig()
	if err != nil {
		return "", err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return "", err
	}
	// Find the ClusterExtension GVR
	disc, err := o.kubernetes.ToDiscoveryClient()
	if err != nil {
		return "", err
	}
	lists, err := disc.ServerPreferredResources()
	if err != nil {
		// continue if partial
	}
	var gvr schema.GroupVersionResource
	found := false
	for _, apiList := range lists {
		gv, _ := schema.ParseGroupVersion(apiList.GroupVersion)
		for _, r := range apiList.APIResources {
			if strings.EqualFold(r.Kind, "ClusterExtension") || r.Name == "clusterextensions" {
				gvr = gv.WithResource(r.Name)
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return "", fmt.Errorf("ClusterExtension resource not found on the cluster")
	}

	existing, err := dyn.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", fmt.Errorf("ClusterExtension %s not found", name)
	} else if err != nil {
		return "", err
	}

	u, err := parseManifest(manifest)
	if err != nil {
		return "", err
	}
	// Ensure name and resourceVersion are set from existing to perform update
	u.SetName(name)
	if existing.GetNamespace() != "" {
		u.SetNamespace(existing.GetNamespace())
	}
	u.SetResourceVersion(existing.GetResourceVersion())

	var updated *unstructured.Unstructured
	if existing.GetNamespace() != "" {
		updated, err = dyn.Resource(gvr).Namespace(existing.GetNamespace()).Update(ctx, u, metav1.UpdateOptions{})
	} else {
		updated, err = dyn.Resource(gvr).Update(ctx, u, metav1.UpdateOptions{})
	}
	if err != nil {
		return "", err
	}
	out, err := yaml.Marshal(updated.Object)
	if err != nil {
		b, _ := json.Marshal(updated.Object)
		return string(b), nil
	}
	return string(out), nil
}
