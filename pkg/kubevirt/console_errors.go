package kubevirt

import "fmt"

type ConsoleErrorCode string

const (
	ConsoleCodeVMINotFound           ConsoleErrorCode = "vmi_not_found"
	ConsoleCodeVMINotRunning         ConsoleErrorCode = "vmi_not_running"
	ConsoleCodeGraphicsDisabled      ConsoleErrorCode = "graphics_disabled"
	ConsoleCodeScreenshotUnavailable ConsoleErrorCode = "screenshot_unavailable"
	ConsoleCodePermissionDenied      ConsoleErrorCode = "permission_denied"
	ConsoleCodeInternal              ConsoleErrorCode = "internal"
)

type ConsoleError struct {
	Code ConsoleErrorCode
	Err  error
}

func (e *ConsoleError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Err)
}

func (e *ConsoleError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
