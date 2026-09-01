package kubevirt

import "fmt"

type ConsoleErrorCode string

const (
	ConsoleCodeVMINotFound           ConsoleErrorCode = "vmi_not_found"
	ConsoleCodeVMINotRunning         ConsoleErrorCode = "vmi_not_running"
	ConsoleCodeGraphicsDisabled      ConsoleErrorCode = "graphics_disabled"
	ConsoleCodeScreenshotTooLarge    ConsoleErrorCode = "screenshot_too_large"
	ConsoleCodeScreenshotUnavailable ConsoleErrorCode = "screenshot_unavailable"
	ConsoleCodePermissionDenied      ConsoleErrorCode = "permission_denied"
	ConsoleCodeInternal              ConsoleErrorCode = "internal"
)

type ConsoleError struct {
	Code    ConsoleErrorCode
	Message string
	Cause   error
}

func (e *ConsoleError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *ConsoleError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
