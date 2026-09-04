package kubevirt

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

func getVMI(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (*unstructured.Unstructured, error) {
	vmi, err := dynamicClient.Resource(VirtualMachineInstanceGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		switch {
		case apierrors.IsNotFound(err):
			return nil, &ConsoleError{
				Code:    ConsoleCodeVMINotFound,
				Message: fmt.Sprintf("VirtualMachineInstance %q not found in namespace %q", name, namespace),
			}
		case apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err):
			return nil, &ConsoleError{Code: ConsoleCodePermissionDenied, Message: "permission denied accessing the VirtualMachineInstance"}
		default:
			return nil, &ConsoleError{Code: ConsoleCodeInternal, Message: "failed to retrieve the VirtualMachineInstance", Cause: err}
		}
	}
	return vmi, nil
}

func vmiPhase(vmi *unstructured.Unstructured) string {
	if vmi == nil {
		return ""
	}
	phase, _, _ := unstructured.NestedString(vmi.Object, "status", "phase")
	return phase
}

func graphicsEnabled(vmi *unstructured.Unstructured) bool {
	if vmi == nil {
		return false
	}
	enabled, found, err := unstructured.NestedBool(vmi.Object, "spec", "domain", "devices", "autoattachGraphicsDevice")
	if err != nil || !found {
		return true
	}
	return enabled
}
