package kubevirt

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

// CreateVMFromTemplate calls the create subresource on a VirtualMachineTemplate
// to perform server-side parameter substitution and create the resulting
// VirtualMachine in a single atomic operation.
func CreateVMFromTemplate(ctx context.Context, restConfig *rest.Config, namespace, templateName string, parameters map[string]string) (map[string]any, error) {
	config := rest.CopyConfig(restConfig)

	gv := schema.GroupVersion{Group: "subresources.template.kubevirt.io", Version: "v1beta1"}
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = subresourcesCodec.WithoutConversion()

	restClient, err := rest.RESTClientFor(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST client for template subresources: %w", err)
	}

	createOptions := map[string]any{
		"apiVersion": "subresources.template.kubevirt.io/v1beta1",
		"kind":       "CreateOptions",
	}
	if len(parameters) > 0 {
		createOptions["parameters"] = parameters
	}

	body, err := json.Marshal(createOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal CreateOptions: %w", err)
	}

	result := &unstructured.Unstructured{}
	err = restClient.Post().
		Namespace(namespace).
		Resource("virtualmachinetemplates").
		Name(templateName).
		SubResource("create").
		Body(body).
		Do(ctx).
		Into(result)

	if err != nil {
		return nil, fmt.Errorf("failed to create VM from template %s/%s: %w", namespace, templateName, err)
	}

	return result.Object, nil
}
