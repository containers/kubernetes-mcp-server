package kubevirt

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// KubeVirt core resources
var (
	// VirtualMachineGVK is the GroupVersionKind for VirtualMachine resources
	VirtualMachineGVK = schema.GroupVersionKind{
		Group:   "kubevirt.io",
		Version: "v1",
		Kind:    "VirtualMachine",
	}

	// VirtualMachineGVR is the GroupVersionResource for VirtualMachine resources
	VirtualMachineGVR = schema.GroupVersionResource{
		Group:    "kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachines",
	}

	// VirtualMachineInstanceGVR is the GroupVersionResource for VirtualMachineInstance resources
	VirtualMachineInstanceGVR = schema.GroupVersionResource{
		Group:    "kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachineinstances",
	}
)

// CDI (Containerized Data Importer) resources
var (
	// DataVolumeGVR is the GroupVersionResource for DataVolume resources
	DataVolumeGVR = schema.GroupVersionResource{
		Group:    "cdi.kubevirt.io",
		Version:  "v1beta1",
		Resource: "datavolumes",
	}

	// DataSourceGVR is the GroupVersionResource for DataSource resources
	DataSourceGVR = schema.GroupVersionResource{
		Group:    "cdi.kubevirt.io",
		Version:  "v1beta1",
		Resource: "datasources",
	}
)

// Instancetype resources
var (
	// VirtualMachineClusterInstancetypeGVR is the GroupVersionResource for cluster-scoped VirtualMachineClusterInstancetype resources
	VirtualMachineClusterInstancetypeGVR = schema.GroupVersionResource{
		Group:    "instancetype.kubevirt.io",
		Version:  "v1beta1",
		Resource: "virtualmachineclusterinstancetypes",
	}

	// VirtualMachineInstancetypeGVR is the GroupVersionResource for namespaced VirtualMachineInstancetype resources
	VirtualMachineInstancetypeGVR = schema.GroupVersionResource{
		Group:    "instancetype.kubevirt.io",
		Version:  "v1beta1",
		Resource: "virtualmachineinstancetypes",
	}
)

// Preference resources
var (
	// VirtualMachineClusterPreferenceGVR is the GroupVersionResource for cluster-scoped VirtualMachineClusterPreference resources
	VirtualMachineClusterPreferenceGVR = schema.GroupVersionResource{
		Group:    "instancetype.kubevirt.io",
		Version:  "v1beta1",
		Resource: "virtualmachineclusterpreferences",
	}

	// VirtualMachinePreferenceGVR is the GroupVersionResource for namespaced VirtualMachinePreference resources
	VirtualMachinePreferenceGVR = schema.GroupVersionResource{
		Group:    "instancetype.kubevirt.io",
		Version:  "v1beta1",
		Resource: "virtualmachinepreferences",
	}
)

// Kubernetes core resources
var (
	// PersistentVolumeClaimGVR is the GroupVersionResource for PersistentVolumeClaim resources
	PersistentVolumeClaimGVR = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "persistentvolumeclaims",
	}

	// PodGVR is the GroupVersionResource for Pod resources
	PodGVR = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}
)
