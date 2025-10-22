package testing

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// NewUnstructuredInstancetype creates a test VirtualMachineClusterInstancetype
func NewUnstructuredInstancetype(name string, labels map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	labelsInterface := make(map[string]interface{})
	for k, v := range labels {
		labelsInterface[k] = v
	}

	metadata := map[string]interface{}{
		"name": name,
	}
	if len(labelsInterface) > 0 {
		metadata["labels"] = labelsInterface
	}

	obj.SetUnstructuredContent(map[string]interface{}{
		"apiVersion": "instancetype.kubevirt.io/v1beta1",
		"kind":       "VirtualMachineClusterInstancetype",
		"metadata":   metadata,
	})
	return obj
}

// NewUnstructuredPreference creates a test VirtualMachinePreference or VirtualMachineClusterPreference
func NewUnstructuredPreference(name string, namespaced bool) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	kind := "VirtualMachineClusterPreference"
	if namespaced {
		kind = "VirtualMachinePreference"
	}
	obj.SetUnstructuredContent(map[string]interface{}{
		"apiVersion": "instancetype.kubevirt.io/v1beta1",
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name": name,
		},
	})
	if namespaced {
		obj.SetNamespace("test-ns")
	}
	return obj
}

// NewUnstructuredDataSource creates a test DataSource with optional default instancetype and preference
func NewUnstructuredDataSource(name, namespace, imageURL, defaultInstancetype, defaultPreference string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	labels := map[string]interface{}{}
	if defaultInstancetype != "" {
		labels["instancetype.kubevirt.io/default-instancetype"] = defaultInstancetype
	}
	if defaultPreference != "" {
		labels["instancetype.kubevirt.io/default-preference"] = defaultPreference
	}

	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if len(labels) > 0 {
		metadata["labels"] = labels
	}

	obj.SetUnstructuredContent(map[string]interface{}{
		"apiVersion": "cdi.kubevirt.io/v1beta1",
		"kind":       "DataSource",
		"metadata":   metadata,
		"spec": map[string]interface{}{
			"source": map[string]interface{}{
				"registry": map[string]interface{}{
					"url": imageURL,
				},
			},
		},
	})
	return obj
}
