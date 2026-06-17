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

// TestSIGHUPIgnored verifies that a SIGHUP signal does not shut down the HTTP
// server. Serve drains SIGHUP with a no-op so that the no-config HTTP path
// (where cmd/root.go does not register a configuration-reload handler) keeps
// the documented "SIGHUP signals are ignored" behavior, instead of letting
// Go's default disposition terminate the process abruptly and bypass graceful
// shutdown (no httpServer.Shutdown / mcpServer.Shutdown, no metrics flush).
//
// When a config file IS present, cmd/root.go registers its own SIGHUP handler;
// signal.Notify multicasts, so that handler still receives its own copy and
// performs the reload while this drain remains a harmless no-op.
func TestSIGHUPIgnored(t *testing.T) {
	testCase(t, func(ctx *httpContext) {
		// beforeEach has already started Serve and waited for it to listen, so
		// the SIGHUP drain is registered before we raise the signal.
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
			t.Fatalf("failed to send SIGHUP: %v", err)
		}

		// The primary regression guard is implicit: if Serve did not drain
		// SIGHUP, Go's default disposition would terminate this whole test binary
		// right here (manifesting as "signal: hangup"), so merely reaching the
		// assertions below proves the process survived. Do NOT remove the SIGHUP
		// drain in Serve expecting these in-body checks to cover it — they only
		// catch the secondary case where SIGHUP wrongly drives the graceful
		// shutdown path while the process happens to survive.
		//
		// Poll the negative condition over a short window (rather than sleeping a
		// fixed amount and sampling once): the server must keep serving and must
		// not log any shutdown activity. Transient GET errors are tolerated so a
		// single localhost hiccup cannot flake the test; we only require that the
		// server answered /healthz at least once and never logged a shutdown.
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
