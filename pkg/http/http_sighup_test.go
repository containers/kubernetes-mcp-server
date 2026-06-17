//go:build !windows

package http

import (
	"fmt"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestSIGHUPIgnored verifies SIGHUP does not shut down the HTTP server. The real
// guard is implicit: without Serve's SIGHUP registration Go's default
// disposition would kill this test binary outright ("signal: hangup"), so
// reaching the assertions proves survival. The in-body checks only catch the
// secondary case where SIGHUP wrongly drives graceful shutdown but the process
// survives, so don't drop the SIGHUP handling expecting them to cover it.
func TestSIGHUPIgnored(t *testing.T) {
	testCase(t, func(ctx *httpContext) {
		// beforeEach waited for Serve to listen, so SIGHUP is registered first.
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
			t.Fatalf("failed to send SIGHUP: %v", err)
		}

		// Poll the negative over a window: keep serving, never log a shutdown.
		// Tolerate transient GET errors (a localhost hiccup must not flake); just
		// require at least one 200 and no shutdown line.
		servedOK := false
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			if logOutput := ctx.LogBuffer.String(); strings.Contains(logOutput, "initiating graceful shutdown") ||
				strings.Contains(logOutput, "Shutting down HTTP server") {
				t.Fatalf("SIGHUP must not trigger shutdown, got log: %s", logOutput)
			}
			if resp, err := http.Get(fmt.Sprintf("http://%s/healthz", ctx.HttpAddress)); err == nil {
				if resp.StatusCode == http.StatusOK {
					servedOK = true
				}
				_ = resp.Body.Close()
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !servedOK {
			t.Errorf("server should have kept serving /healthz after SIGHUP, but no 200 response was observed")
		}
	})
}
