package redact

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// VMCloudInitFields replaces inline cloud-init payloads in a VirtualMachine
// object with "[REDACTED]". This prevents passwords, SSH keys, and other
// credentials embedded in userData/networkData from appearing in LLM context
// windows and audit logs. The object is modified in-place; the function is a
// no-op for objects that have no cloud-init volumes.
func VMCloudInitFields(vm *unstructured.Unstructured) {
	if vm == nil {
		return
	}
	volumes, found, err := unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "volumes")
	if err != nil || !found {
		return
	}
	changed := false
	for i, vol := range volumes {
		volMap, ok := vol.(map[string]interface{})
		if !ok {
			continue
		}
		for _, ciKey := range []string{"cloudInitNoCloud", "cloudInitConfigDrive"} {
			ciData, ok := volMap[ciKey].(map[string]interface{})
			if !ok {
				continue
			}
			for _, field := range []string{"userData", "userDataBase64", "networkData", "networkDataBase64"} {
				if _, has := ciData[field]; has {
					ciData[field] = "[REDACTED]"
					changed = true
				}
			}
		}
		volumes[i] = volMap
	}
	if changed {
		_ = unstructured.SetNestedSlice(vm.Object, volumes, "spec", "template", "spec", "volumes")
	}
}
