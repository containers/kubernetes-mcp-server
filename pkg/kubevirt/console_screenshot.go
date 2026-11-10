package kubevirt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// Screenshot captures the current contents of a VirtualMachineInstance's
// graphical (VNC) console and returns the raw PNG bytes along with the decoded
// image dimensions.
func Screenshot(ctx context.Context, dynamicClient dynamic.Interface, restConfig *rest.Config, namespace, name string) ([]byte, image.Config, error) {
	vmi, err := getVMI(ctx, dynamicClient, namespace, name)
	if err != nil {
		return nil, image.Config{}, err
	}

	if err := validateScreenshotEligible(vmi, name); err != nil {
		return nil, image.Config{}, err
	}

	data, err := fetchScreenshot(ctx, restConfig, namespace, name)
	if err != nil {
		return nil, image.Config{}, err
	}

	cfg, err := decodePNGConfig(data)
	if err != nil {
		return nil, image.Config{}, err
	}

	return data, cfg, nil
}

func validateScreenshotEligible(vmi *unstructured.Unstructured, name string) error {
	if phase := vmiPhase(vmi); phase != "Running" {
		return &ConsoleError{
			Code: ConsoleCodeVMINotRunning,
			Err:  fmt.Errorf("VirtualMachineInstance %q is in phase %q, not Running", name, phase),
		}
	}

	if !graphicsEnabled(vmi) {
		return &ConsoleError{
			Code: ConsoleCodeGraphicsDisabled,
			Err:  fmt.Errorf("VirtualMachineInstance %q has no graphical console (autoattachGraphicsDevice is false)", name),
		}
	}

	return nil
}

func fetchScreenshot(ctx context.Context, restConfig *rest.Config, namespace, name string) ([]byte, error) {
	client, err := newSubresourceClient(restConfig)
	if err != nil {
		return nil, &ConsoleError{Code: ConsoleCodeInternal, Err: fmt.Errorf("failed to create subresource client: %w", err)}
	}

	stream, err := client.Get().
		Namespace(namespace).
		Resource("virtualmachineinstances").
		Name(name).
		SubResource("vnc", "screenshot").
		Param("moveCursor", "true").
		Stream(ctx)
	if err != nil {
		return nil, mapScreenshotStreamError(err)
	}
	defer func() { _ = stream.Close() }()

	// Size is capped by max_backend_response_bytes on the shared round tripper.
	data, err := io.ReadAll(stream)
	if err != nil {
		var tooLarge *kubernetes.BackendResponseTooLargeError
		if errors.As(err, &tooLarge) {
			return nil, tooLarge
		}
		return nil, &ConsoleError{Code: ConsoleCodeScreenshotUnavailable, Err: fmt.Errorf("failed to read screenshot data: %w", err)}
	}

	if len(data) == 0 {
		return nil, &ConsoleError{Code: ConsoleCodeScreenshotUnavailable, Err: fmt.Errorf("the VNC screenshot subresource returned no data")}
	}

	return data, nil
}

func decodePNGConfig(data []byte) (image.Config, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return image.Config{}, &ConsoleError{
			Code: ConsoleCodeScreenshotUnavailable,
			Err:  fmt.Errorf("the screenshot subresource did not return a valid PNG image: %w", err),
		}
	}
	return cfg, nil
}

func mapScreenshotStreamError(err error) error {
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return &ConsoleError{Code: ConsoleCodePermissionDenied, Err: fmt.Errorf("permission denied accessing the VNC screenshot subresource: %w", err)}
	}
	return &ConsoleError{Code: ConsoleCodeScreenshotUnavailable, Err: fmt.Errorf("failed to fetch the screenshot from the VNC subresource: %w", err)}
}
