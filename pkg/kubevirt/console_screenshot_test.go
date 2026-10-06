package kubevirt

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

type ConsoleScreenshotSuite struct {
	suite.Suite
	ctx context.Context
}

func (s *ConsoleScreenshotSuite) SetupTest() {
	s.ctx = context.Background()
}

func newTestVMI(namespace, name, phase string, autoattachGraphics *bool) *unstructured.Unstructured {
	devices := map[string]any{}
	if autoattachGraphics != nil {
		devices["autoattachGraphicsDevice"] = *autoattachGraphics
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "VirtualMachineInstance",
		"metadata": map[string]any{
			"namespace": namespace,
			"name":      name,
		},
		"spec": map[string]any{
			"domain": map[string]any{
				"devices": devices,
			},
		},
		"status": map[string]any{
			"phase": phase,
		},
	}}
}

func newFakeDynamicClient(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{
		VirtualMachineInstanceGVR: "VirtualMachineInstanceList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objs...)
}

// requireConsoleCode asserts that err is a *ConsoleError carrying the wanted code.
func (s *ConsoleScreenshotSuite) requireConsoleCode(err error, want ConsoleErrorCode) {
	s.T().Helper()
	s.Require().Error(err, "expected an error with code %q", want)
	var ce *ConsoleError
	s.Require().ErrorAs(err, &ce, "expected a *ConsoleError")
	s.Equal(want, ce.Code, "unexpected console error code (%s)", ce.Err)
}

func (s *ConsoleScreenshotSuite) validPNG() []byte {
	s.T().Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	s.Require().NoError(png.Encode(&buf, img))
	return buf.Bytes()
}

func (s *ConsoleScreenshotSuite) screenshotTestServer(handler http.HandlerFunc) *rest.Config {
	s.T().Helper()
	server := httptest.NewServer(handler)
	s.T().Cleanup(server.Close)
	return &rest.Config{Host: server.URL}
}

func (s *ConsoleScreenshotSuite) TestSuccess() {
	s.Run("returns the PNG bytes and decoded image config", func() {
		expected := s.validPNG()
		cfg := s.screenshotTestServer(func(w http.ResponseWriter, r *http.Request) {
			s.True(strings.HasSuffix(r.URL.Path, "/vnc/screenshot"), "unexpected path %q", r.URL.Path)
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(expected)
		})
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", nil))

		data, imgCfg, err := Screenshot(s.ctx, dyn, cfg, "default", "test-vm")
		s.Require().NoError(err)
		s.Equal(expected, data)
		s.Equal(2, imgCfg.Width)
		s.Equal(2, imgCfg.Height)
	})

	s.Run("passes moveCursor=true to wake a blanked display before capturing", func() {
		expected := s.validPNG()
		cfg := s.screenshotTestServer(func(w http.ResponseWriter, r *http.Request) {
			s.Equal("true", r.URL.Query().Get("moveCursor"))
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(expected)
		})
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", nil))

		_, _, err := Screenshot(s.ctx, dyn, cfg, "default", "test-vm")
		s.Require().NoError(err)
	})

	s.Run("accepts a screenshot exactly at the size limit", func() {
		padded := make([]byte, ConsoleMaxScreenshotBytes)
		copy(padded, s.validPNG()) // valid PNG header, trailing zero padding tolerated by DecodeConfig
		cfg := s.screenshotTestServer(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(padded)
		})
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", nil))

		data, _, err := Screenshot(s.ctx, dyn, cfg, "default", "test-vm")
		s.Require().NoError(err)
		s.Len(data, ConsoleMaxScreenshotBytes)
	})
}

func (s *ConsoleScreenshotSuite) TestVMIValidation() {
	s.Run("VMI not found", func() {
		dyn := newFakeDynamicClient()
		_, _, err := Screenshot(s.ctx, dyn, &rest.Config{}, "default", "missing")
		s.requireConsoleCode(err, ConsoleCodeVMINotFound)
	})

	s.Run("no permission to read the VMI", func() {
		dyn := newFakeDynamicClient()
		dyn.PrependReactor("get", "virtualmachineinstances", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(
				schema.GroupResource{Group: "kubevirt.io", Resource: "virtualmachineinstances"},
				"test-vm", errors.New("forbidden"))
		})
		_, _, err := Screenshot(s.ctx, dyn, &rest.Config{}, "default", "test-vm")
		s.requireConsoleCode(err, ConsoleCodePermissionDenied)
	})

	s.Run("VMI not running", func() {
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Pending", nil))
		_, _, err := Screenshot(s.ctx, dyn, &rest.Config{}, "default", "test-vm")
		s.requireConsoleCode(err, ConsoleCodeVMINotRunning)
	})

	s.Run("graphics disabled", func() {
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", ptr.To(false)))
		_, _, err := Screenshot(s.ctx, dyn, &rest.Config{}, "default", "test-vm")
		s.requireConsoleCode(err, ConsoleCodeGraphicsDisabled)
	})

	s.Run("graphics enabled when explicitly true", func() {
		expected := s.validPNG()
		cfg := s.screenshotTestServer(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(expected)
		})
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", ptr.To(true)))
		_, _, err := Screenshot(s.ctx, dyn, cfg, "default", "test-vm")
		s.Require().NoError(err)
	})
}

func (s *ConsoleScreenshotSuite) TestScreenshotResponseErrors() {
	s.Run("rejects an oversized screenshot", func() {
		cfg := s.screenshotTestServer(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(make([]byte, ConsoleMaxScreenshotBytes+1))
		})
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", nil))
		_, _, err := Screenshot(s.ctx, dyn, cfg, "default", "test-vm")
		s.requireConsoleCode(err, ConsoleCodeScreenshotTooLarge)
	})

	s.Run("rejects an empty response", func() {
		cfg := s.screenshotTestServer(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
		})
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", nil))
		_, _, err := Screenshot(s.ctx, dyn, cfg, "default", "test-vm")
		s.requireConsoleCode(err, ConsoleCodeScreenshotUnavailable)
	})

	s.Run("rejects non-PNG data", func() {
		cfg := s.screenshotTestServer(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("this is not a png"))
		})
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", nil))
		_, _, err := Screenshot(s.ctx, dyn, cfg, "default", "test-vm")
		s.requireConsoleCode(err, ConsoleCodeScreenshotUnavailable)
	})

	s.Run("maps a forbidden subresource response to permission denied", func() {
		cfg := s.screenshotTestServer(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})
		dyn := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", nil))
		_, _, err := Screenshot(s.ctx, dyn, cfg, "default", "test-vm")
		s.requireConsoleCode(err, ConsoleCodePermissionDenied)
	})
}

func TestConsoleScreenshotSuite(t *testing.T) {
	suite.Run(t, new(ConsoleScreenshotSuite))
}
