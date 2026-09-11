package redact

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func vmWithVolumes(volumes []interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": volumes,
					},
				},
			},
		},
	}
}

func TestVMCloudInitFields_NilVM(t *testing.T) {
	// Should not panic
	VMCloudInitFields(nil)
}

func TestVMCloudInitFields_NoVolumes(t *testing.T) {
	vm := &unstructured.Unstructured{Object: map[string]interface{}{}}
	VMCloudInitFields(vm) // no-op, must not panic
}

func TestVMCloudInitFields_NoCloudInit(t *testing.T) {
	vm := vmWithVolumes([]interface{}{
		map[string]interface{}{
			"name":                  "rootdisk",
			"containerDisk":         map[string]interface{}{"image": "quay.io/containerdisks/fedora:latest"},
		},
	})
	VMCloudInitFields(vm)
	vols, _, _ := unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "volumes")
	disk := vols[0].(map[string]interface{})
	if disk["containerDisk"] == nil {
		t.Fatal("containerDisk unexpectedly removed")
	}
}

func TestVMCloudInitFields_CloudInitNoCloud_Plain(t *testing.T) {
	vm := vmWithVolumes([]interface{}{
		map[string]interface{}{
			"name": "cloudinitdisk",
			"cloudInitNoCloud": map[string]interface{}{
				"userData":    "#cloud-config\npassword: secret\n",
				"networkData": "version: 2\n",
			},
		},
	})
	VMCloudInitFields(vm)
	vols, _, _ := unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "volumes")
	ci := vols[0].(map[string]interface{})["cloudInitNoCloud"].(map[string]interface{})
	if ci["userData"] != "[REDACTED]" {
		t.Errorf("userData not redacted: %v", ci["userData"])
	}
	if ci["networkData"] != "[REDACTED]" {
		t.Errorf("networkData not redacted: %v", ci["networkData"])
	}
}

func TestVMCloudInitFields_CloudInitNoCloud_Base64(t *testing.T) {
	vm := vmWithVolumes([]interface{}{
		map[string]interface{}{
			"name": "cloudinitdisk",
			"cloudInitNoCloud": map[string]interface{}{
				"userDataBase64":    "I2Nsb3VkLWNvbmZpZwpwYXNzd29yZDogc2VjcmV0Cg==",
				"networkDataBase64": "dmVyc2lvbjogMgo=",
			},
		},
	})
	VMCloudInitFields(vm)
	vols, _, _ := unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "volumes")
	ci := vols[0].(map[string]interface{})["cloudInitNoCloud"].(map[string]interface{})
	if ci["userDataBase64"] != "[REDACTED]" {
		t.Errorf("userDataBase64 not redacted: %v", ci["userDataBase64"])
	}
	if ci["networkDataBase64"] != "[REDACTED]" {
		t.Errorf("networkDataBase64 not redacted: %v", ci["networkDataBase64"])
	}
}

func TestVMCloudInitFields_CloudInitConfigDrive(t *testing.T) {
	vm := vmWithVolumes([]interface{}{
		map[string]interface{}{
			"name": "cloudinitdisk",
			"cloudInitConfigDrive": map[string]interface{}{
				"userData": "#cloud-config\nssh_authorized_keys:\n  - ssh-rsa AAAA...\n",
			},
		},
	})
	VMCloudInitFields(vm)
	vols, _, _ := unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "volumes")
	ci := vols[0].(map[string]interface{})["cloudInitConfigDrive"].(map[string]interface{})
	if ci["userData"] != "[REDACTED]" {
		t.Errorf("userData not redacted: %v", ci["userData"])
	}
}

func TestVMCloudInitFields_PreservesOtherFields(t *testing.T) {
	vm := vmWithVolumes([]interface{}{
		map[string]interface{}{
			"name": "cloudinitdisk",
			"cloudInitNoCloud": map[string]interface{}{
				"userData":          "#cloud-config\npassword: secret\n",
				"userDataSecretRef": map[string]interface{}{"name": "my-secret"},
			},
		},
	})
	VMCloudInitFields(vm)
	vols, _, _ := unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "volumes")
	ci := vols[0].(map[string]interface{})["cloudInitNoCloud"].(map[string]interface{})
	if ci["userData"] != "[REDACTED]" {
		t.Errorf("userData not redacted: %v", ci["userData"])
	}
	// userDataSecretRef is a reference, not a payload — must be preserved
	if ci["userDataSecretRef"] == nil {
		t.Error("userDataSecretRef unexpectedly removed")
	}
}

func TestVMCloudInitFields_MultipleVolumes(t *testing.T) {
	vm := vmWithVolumes([]interface{}{
		map[string]interface{}{
			"name":          "rootdisk",
			"containerDisk": map[string]interface{}{"image": "quay.io/containerdisks/fedora:latest"},
		},
		map[string]interface{}{
			"name": "cloudinitdisk",
			"cloudInitNoCloud": map[string]interface{}{
				"userData": "#cloud-config\npassword: secret\n",
			},
		},
	})
	VMCloudInitFields(vm)
	vols, _, _ := unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "volumes")
	// First volume untouched
	root := vols[0].(map[string]interface{})
	if root["containerDisk"] == nil {
		t.Error("containerDisk unexpectedly removed from rootdisk volume")
	}
	// Second volume redacted
	ci := vols[1].(map[string]interface{})["cloudInitNoCloud"].(map[string]interface{})
	if ci["userData"] != "[REDACTED]" {
		t.Errorf("userData not redacted: %v", ci["userData"])
	}
}
