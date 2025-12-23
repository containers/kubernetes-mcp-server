package mcp

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/containers/kubernetes-mcp-server/internal/test"
)

type PodsExecSuite struct {
	BaseMcpSuite
	mockServer *test.MockServer
}

func (s *PodsExecSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
	s.Cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())
}

func (s *PodsExecSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *PodsExecSuite) TestPodsExec() {
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/v1/namespaces/default/pods/pod-to-exec/exec" {
			return
		}
		var stdin, stdout bytes.Buffer
		ctx, err := test.CreateHTTPStreams(w, req, &test.StreamOptions{
			Stdin:  &stdin,
			Stdout: &stdout,
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		defer func(conn io.Closer) { _ = conn.Close() }(ctx.Closer)
		_, _ = io.WriteString(ctx.StdoutStream, "command:"+strings.Join(req.URL.Query()["command"], " ")+"\n")
		_, _ = io.WriteString(ctx.StdoutStream, "container:"+strings.Join(req.URL.Query()["container"], " ")+"\n")
	}))
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/v1/namespaces/default/pods/pod-to-exec" {
			return
		}
		test.WriteObject(w, &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "pod-to-exec",
			},
			Spec: v1.PodSpec{Containers: []v1.Container{{Name: "container-to-exec"}}},
		})
	}))
	s.InitMcpClient()

	s.Run("pods_exec(name=pod-to-exec, namespace=nil, command=[ls -l]), uses configured namespace", func() {
		result, err := s.CallTool("pods_exec", map[string]interface{}{
			"name":    "pod-to-exec",
			"command": []interface{}{"ls", "-l"},
		})
		s.Require().NotNil(result)
		s.Run("returns command output", func() {
			s.NoError(err, "call tool failed %v", err)
			s.Falsef(result.IsError, "call tool failed: %v", result.Content)
			s.Contains(result.Content[0].(mcp.TextContent).Text, "command:ls -l\n", "unexpected result %v", result.Content[0].(mcp.TextContent).Text)
		})
	})
	s.Run("pods_exec(name=pod-to-exec, namespace=default, command=[ls -l])", func() {
		result, err := s.CallTool("pods_exec", map[string]interface{}{
			"namespace": "default",
			"name":      "pod-to-exec",
			"command":   []interface{}{"ls", "-l"},
		})
		s.Require().NotNil(result)
		s.Run("returns command output", func() {
			s.NoError(err, "call tool failed %v", err)
			s.Falsef(result.IsError, "call tool failed: %v", result.Content)
			s.Contains(result.Content[0].(mcp.TextContent).Text, "command:ls -l\n", "unexpected result %v", result.Content[0].(mcp.TextContent).Text)
		})
	})
	s.Run("pods_exec(name=pod-to-exec, namespace=default, command=[ls -l], container=a-specific-container)", func() {
		result, err := s.CallTool("pods_exec", map[string]interface{}{
			"namespace": "default",
			"name":      "pod-to-exec",
			"command":   []interface{}{"ls", "-l"},
			"container": "a-specific-container",
		})
		s.Require().NotNil(result)
		s.Run("returns command output", func() {
			s.NoError(err, "call tool failed %v", err)
			s.Falsef(result.IsError, "call tool failed: %v", result.Content)
			s.Contains(result.Content[0].(mcp.TextContent).Text, "command:ls -l\n", "unexpected result %v", result.Content[0].(mcp.TextContent).Text)
		})
	})
}

func (s *PodsExecSuite) TestPodsExecDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { version = "v1", kind = "Pod" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	s.Run("pods_exec (denied)", func() {
		toolResult, err := s.CallTool("pods_exec", map[string]interface{}{
			"namespace": "default",
			"name":      "pod-to-exec",
			"command":   []interface{}{"ls", "-l"},
			"container": "a-specific-container",
		})
		s.Require().NotNil(toolResult, "toolResult should not be nil")
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := toolResult.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to exec in pod pod-to-exec in namespace default:(.+:)? resource not allowed: /v1, Kind=Pod"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
}

func TestPodsExec(t *testing.T) {
	suite.Run(t, new(PodsExecSuite))
}
