package kubevirt

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"io"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// ConsoleMaxScreenshotBytes bounds the size of a VNC screenshot PNG read into
// memory. Responses larger than this are rejected rather than buffered whole.
const ConsoleMaxScreenshotBytes = 4 * 1024 * 1024 // 4 MiB

func Screenshot(ctx context.Context, dynamicClient dynamic.Interface, restConfig *rest.Config, namespace, name string, wakeScreen bool) ([]byte, string, error) {
	vmi, err := getVMI(ctx, dynamicClient, namespace, name)
	if err != nil {
		return nil, "", err
	}

	if phase := vmiPhase(vmi); phase != "Running" {
		return nil, "", &ConsoleError{
			Code:    ConsoleCodeVMINotRunning,
			Message: fmt.Sprintf("VirtualMachineInstance %q is in phase %q, not Running", name, phase),
		}
	}

	if !graphicsEnabled(vmi) {
		return nil, "", &ConsoleError{
			Code:    ConsoleCodeGraphicsDisabled,
			Message: fmt.Sprintf("VirtualMachineInstance %q has no graphical console (autoattachGraphicsDevice is false)", name),
		}
	}

	client, err := newSubresourceClient(restConfig)
	if err != nil {
		return nil, "", &ConsoleError{Code: ConsoleCodeInternal, Message: "failed to create subresource client", Cause: err}
	}

	stream, err := client.Get().
		Namespace(namespace).
		Resource("virtualmachineinstances").
		Name(name).
		SubResource("vnc", "screenshot").
		Param("moveCursor", strconv.FormatBool(wakeScreen)).
		Stream(ctx)
	if err != nil {
		return nil, "", mapScreenshotStreamError(err)
	}
	defer func() { _ = stream.Close() }()

	// Read one byte past the limit so a response exactly at the limit still
	// succeeds while anything larger is detected as truncated and rejected.
	data, err := io.ReadAll(io.LimitReader(stream, ConsoleMaxScreenshotBytes+1))
	if err != nil {
		return nil, "", &ConsoleError{Code: ConsoleCodeScreenshotUnavailable, Message: "failed to read screenshot data", Cause: err}
	}
	if len(data) > ConsoleMaxScreenshotBytes {
		return nil, "", &ConsoleError{
			Code:    ConsoleCodeScreenshotTooLarge,
			Message: fmt.Sprintf("screenshot exceeds the maximum allowed size of %d bytes", ConsoleMaxScreenshotBytes),
		}
	}
	if len(data) == 0 {
		return nil, "", &ConsoleError{Code: ConsoleCodeScreenshotUnavailable, Message: "the VNC screenshot subresource returned no data"}
	}

	// Validate the PNG header (IHDR) without decoding the full image; trailing
	// bytes beyond a valid PNG are tolerated by DecodeConfig.
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
		return nil, "", &ConsoleError{Code: ConsoleCodeScreenshotUnavailable, Message: "the screenshot subresource did not return a valid PNG image"}
	}

	return data, "Screenshot captured from the VNC graphical console.", nil
}

func mapScreenshotStreamError(err error) error {
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return &ConsoleError{Code: ConsoleCodePermissionDenied, Message: "permission denied accessing the VNC screenshot subresource", Cause: err}
	}
	return &ConsoleError{Code: ConsoleCodeScreenshotUnavailable, Message: "failed to fetch the screenshot from the VNC subresource", Cause: err}
}
