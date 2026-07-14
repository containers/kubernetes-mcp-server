package kubernetes

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	yml "sigs.k8s.io/yaml"
)

// decodeManifest decodes a YAML or JSON manifest string into a typed
// Kubernetes runtime.Object using the registered scheme. The caller is
// expected to type-assert the result to the concrete kind.
func decodeManifest(manifest string) (runtime.Object, error) {
	if manifest == "" {
		return nil, fmt.Errorf("manifest must not be empty")
	}
	jsonBytes, err := yml.YAMLToJSON([]byte(manifest))
	if err != nil {
		return nil, fmt.Errorf("failed to convert manifest to JSON: %w", err)
	}
	deserializer := serializer.NewCodecFactory(Scheme).UniversalDeserializer()
	obj, _, err := deserializer.Decode(jsonBytes, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}
	return obj, nil
}
