package mcp

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	apiextensionsv1spec "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/utils/ptr"
)

func CRD(group, version, resource, kind, singular string, namespaced bool) *apiextensionsv1spec.CustomResourceDefinition {
	scope := "Cluster"
	if namespaced {
		scope = "Namespaced"
	}
	crd := &apiextensionsv1spec.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1spec.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s.%s", resource, group)},
		Spec: apiextensionsv1spec.CustomResourceDefinitionSpec{
			Group: group,
			Versions: []apiextensionsv1spec.CustomResourceDefinitionVersion{
				{
					Name:    version,
					Served:  false,
					Storage: true,
					Schema: &apiextensionsv1spec.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1spec.JSONSchemaProps{
							Type:                   "object",
							XPreserveUnknownFields: ptr.To(true),
						},
					},
				},
			},
			Scope: apiextensionsv1spec.ResourceScope(scope),
			Names: apiextensionsv1spec.CustomResourceDefinitionNames{
				Plural:     resource,
				Singular:   singular,
				Kind:       kind,
				ShortNames: []string{singular},
			},
		},
	}
	return crd
}

func EnvTestEnableCRD(ctx context.Context, group, version, resource string) error {
	apiExtensionsV1Client := apiextensionsv1.NewForConfigOrDie(envTestRestConfig)
	_, err := apiExtensionsV1Client.CustomResourceDefinitions().Patch(
		ctx,
		fmt.Sprintf("%s.%s", resource, group),
		types.JSONPatchType,
		[]byte(`[{"op": "replace", "path": "/spec/versions/0/served", "value": true}]`),
		metav1.PatchOptions{})
	if err != nil {
		return err
	}
	return EnvTestWaitForAPIResourceCondition(ctx, group, version, resource, true)
}

func EnvTestDisableCRD(ctx context.Context, group, version, resource string) error {
	apiExtensionsV1Client := apiextensionsv1.NewForConfigOrDie(envTestRestConfig)
	_, err := apiExtensionsV1Client.CustomResourceDefinitions().Patch(
		ctx,
		fmt.Sprintf("%s.%s", resource, group),
		types.JSONPatchType,
		[]byte(`[{"op": "replace", "path": "/spec/versions/0/served", "value": false}]`),
		metav1.PatchOptions{})
	if err != nil {
		return err
	}
	return EnvTestWaitForAPIResourceCondition(ctx, group, version, resource, false)
}

func EnvTestWaitForAPIResourceCondition(ctx context.Context, group, version, resource string, shouldBeAvailable bool) error {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(envTestRestConfig)
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}

	groupVersion := fmt.Sprintf("%s/%s", group, version)
	if group == "" {
		groupVersion = version
	}

	return wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 10*time.Second, true, func(ctx context.Context) (bool, error) {
		resourceList, err := discoveryClient.ServerResourcesForGroupVersion(groupVersion)
		if err != nil {
			// If we're waiting for the resource to be unavailable and we get an error, it might be gone
			if !shouldBeAvailable {
				return true, nil
			}
			// Otherwise, keep polling
			return false, nil
		}

		// Check if the resource exists in the list
		found := false
		for _, apiResource := range resourceList.APIResources {
			if apiResource.Name == resource {
				found = true
				break
			}
		}

		// Return true if the condition is met
		if shouldBeAvailable {
			return found, nil
		}
		return !found, nil
	})
}

// EnvTestInOpenShift sets up the kubernetes environment to seem to be running OpenShift
func EnvTestInOpenShift(ctx context.Context) error {
	tasks, _ := errgroup.WithContext(ctx)
	tasks.Go(func() error { return EnvTestEnableCRD(ctx, "project.openshift.io", "v1", "projects") })
	tasks.Go(func() error { return EnvTestEnableCRD(ctx, "route.openshift.io", "v1", "routes") })
	return tasks.Wait()
}

// EnvTestInOpenShiftClear clears the kubernetes environment so it no longer seems to be running OpenShift
func EnvTestInOpenShiftClear(ctx context.Context) error {
	tasks, _ := errgroup.WithContext(ctx)
	tasks.Go(func() error { return EnvTestDisableCRD(ctx, "project.openshift.io", "v1", "projects") })
	tasks.Go(func() error { return EnvTestDisableCRD(ctx, "route.openshift.io", "v1", "routes") })
	return tasks.Wait()
}
