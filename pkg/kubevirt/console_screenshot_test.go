package kubevirt

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/client-go/rest"
)

func validPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode PNG: %v", err)
	}
	return buf.Bytes()
}

func screenshotTestServer(t *testing.T, handler http.HandlerFunc) *rest.Config {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &rest.Config{Host: server.URL}
}

func TestScreenshot(t *testing.T) {
	ctx := context.Background()

	t.Run("returns PNG and description on success", func(t *testing.T) {
		png := validPNG(t)
		cfg := screenshotTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/vnc/screenshot") {
				t.Errorf("expected path ending in /vnc/screenshot, got %q", r.URL.Path)
			}
			if got := r.URL.Query().Get("moveCursor"); got != "false" {
				t.Errorf("expected moveCursor=false, got %q", got)
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		})
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", nil))

		data, desc, err := Screenshot(ctx, dyn, cfg, "default", "test-vm", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(data, png) {
			t.Fatalf("returned PNG bytes differ from server response")
		}
		if desc == "" {
			t.Fatal("expected a non-empty description")
		}
	})

	t.Run("passes moveCursor=true when wakeScreen is set", func(t *testing.T) {
		png := validPNG(t)
		cfg := screenshotTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("moveCursor"); got != "true" {
				t.Errorf("expected moveCursor=true, got %q", got)
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		})
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", nil))

		if _, _, err := Screenshot(ctx, dyn, cfg, "default", "test-vm", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("VMI not found", func(t *testing.T) {
		dyn := newFakeDynamicClient()
		_, _, err := Screenshot(ctx, dyn, &rest.Config{}, "default", "missing", false)
		requireConsoleCode(t, err, ConsoleCodeVMINotFound)
	})

	t.Run("VMI not running", func(t *testing.T) {
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Pending", nil))
		_, _, err := Screenshot(ctx, dyn, &rest.Config{}, "default", "test-vm", false)
		requireConsoleCode(t, err, ConsoleCodeVMINotRunning)
	})

	t.Run("graphics disabled", func(t *testing.T) {
		fls := false
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", &fls))
		_, _, err := Screenshot(ctx, dyn, &rest.Config{}, "default", "test-vm", false)
		requireConsoleCode(t, err, ConsoleCodeGraphicsDisabled)
	})

	t.Run("rejects an oversized screenshot", func(t *testing.T) {
		cfg := screenshotTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(make([]byte, ConsoleMaxScreenshotBytes+1))
		})
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", nil))
		_, _, err := Screenshot(ctx, dyn, cfg, "default", "test-vm", false)
		requireConsoleCode(t, err, ConsoleCodeScreenshotTooLarge)
	})

	t.Run("rejects an empty response", func(t *testing.T) {
		cfg := screenshotTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
		})
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", nil))
		_, _, err := Screenshot(ctx, dyn, cfg, "default", "test-vm", false)
		requireConsoleCode(t, err, ConsoleCodeScreenshotUnavailable)
	})

	t.Run("rejects non-PNG data", func(t *testing.T) {
		cfg := screenshotTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("this is not a png"))
		})
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", nil))
		_, _, err := Screenshot(ctx, dyn, cfg, "default", "test-vm", false)
		requireConsoleCode(t, err, ConsoleCodeScreenshotUnavailable)
	})

	t.Run("accepts a screenshot exactly at the size limit", func(t *testing.T) {
		png := validPNG(t)
		padded := make([]byte, ConsoleMaxScreenshotBytes)
		copy(padded, png) // valid PNG header, trailing zero padding tolerated by DecodeConfig
		cfg := screenshotTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(padded)
		})
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", nil))
		data, _, err := Screenshot(ctx, dyn, cfg, "default", "test-vm", false)
		if err != nil {
			t.Fatalf("unexpected error at exactly the limit: %v", err)
		}
		if len(data) != ConsoleMaxScreenshotBytes {
			t.Fatalf("expected %d bytes, got %d", ConsoleMaxScreenshotBytes, len(data))
		}
	})
}
