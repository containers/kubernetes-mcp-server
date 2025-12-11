package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"flag"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/yaml"
)

type HelmSuite struct {
	BaseMcpSuite
	klogState klog.State
	logBuffer bytes.Buffer
}

func (s *HelmSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	clearHelmReleases(s.T().Context(), kubernetes.NewForConfigOrDie(envTestRestConfig))

	// Capture log output to verify denied resource messages
	s.klogState = klog.CaptureState()
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	klog.InitFlags(flags)
	_ = flags.Set("v", strconv.Itoa(5))
	klog.SetLogger(textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(5), textlogger.Output(&s.logBuffer))))
}

func (s *HelmSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	s.klogState.Restore()
}

func (s *HelmSuite) TestHelmInstall() {
	s.InitMcpClient()
	s.Run("helm_install(chart=helm-chart-no-op)", func() {
		_, file, _, _ := runtime.Caller(0)
		chartPath := filepath.Join(filepath.Dir(file), "testdata", "helm-chart-no-op")
		toolResult, err := s.CallTool("helm_install", map[string]interface{}{
			"chart": chartPath,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("returns installed chart", func() {
			var decoded []map[string]interface{}
			err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
			s.Run("has yaml content", func() {
				s.Nilf(err, "invalid tool result content %v", err)
			})
			s.Run("has 1 item", func() {
				s.Lenf(decoded, 1, "invalid helm install count, expected 1, got %v", len(decoded))
			})
			s.Run("has valid name", func() {
				s.Truef(strings.HasPrefix(decoded[0]["name"].(string), "helm-chart-no-op-"), "invalid helm install name, expected no-op-*, got %v", decoded[0]["name"])
			})
			s.Run("has valid namespace", func() {
				s.Equalf("default", decoded[0]["namespace"], "invalid helm install namespace, expected default, got %v", decoded[0]["namespace"])
			})
			s.Run("has valid chart", func() {
				s.Equalf("no-op", decoded[0]["chart"], "invalid helm install name, expected release name, got empty")
			})
			s.Run("has valid chartVersion", func() {
				s.Equalf("1.33.7", decoded[0]["chartVersion"], "invalid helm install version, expected 1.33.7, got empty")
			})
			s.Run("has valid status", func() {
				s.Equalf("deployed", decoded[0]["status"], "invalid helm install status, expected deployed, got %v", decoded[0]["status"])
			})
			s.Run("has valid revision", func() {
				s.Equalf(float64(1), decoded[0]["revision"], "invalid helm install revision, expected 1, got %v", decoded[0]["revision"])
			})
		})
	})
}

func (s *HelmSuite) TestHelmInstallDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { version = "v1", kind = "Secret" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	s.Run("helm_install(chart=helm-chart-secret, denied)", func() {
		_, file, _, _ := runtime.Caller(0)
		chartPath := filepath.Join(filepath.Dir(file), "testdata", "helm-chart-secret")
		toolResult, err := s.CallTool("helm_install", map[string]interface{}{
			"chart": chartPath,
		})
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := toolResult.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			s.Truef(strings.HasPrefix(msg, "failed to install helm chart"), "expected descriptive error, got %v", msg)
			expectedMessage := ": resource not allowed: /v1, Kind=Secret"
			s.Truef(strings.HasSuffix(msg, expectedMessage), "expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
}

func (s *HelmSuite) TestHelmListNoReleases() {
	s.InitMcpClient()
	s.Run("helm_list() with no releases", func() {
		toolResult, err := s.CallTool("helm_list", map[string]interface{}{})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("returns not found", func() {
			s.Equalf("No Helm releases found", toolResult.Content[0].(mcp.TextContent).Text, "unexpected result %v", toolResult.Content[0].(mcp.TextContent).Text)
		})
	})
}

func (s *HelmSuite) TestHelmList() {
	kc := kubernetes.NewForConfigOrDie(envTestRestConfig)
	_, err := kc.CoreV1().Secrets("default").Create(s.T().Context(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "sh.helm.release.v1.release-to-list",
			Labels: map[string]string{"owner": "helm", "name": "release-to-list"},
		},
		Data: map[string][]byte{
			"release": []byte(base64.StdEncoding.EncodeToString([]byte("{" +
				"\"name\":\"release-to-list\"," +
				"\"info\":{\"status\":\"deployed\"}" +
				"}"))),
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
	s.InitMcpClient()
	s.Run("helm_list() with deployed release", func() {
		toolResult, err := s.CallTool("helm_list", map[string]interface{}{})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("returns release", func() {
			var decoded []map[string]interface{}
			err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
			s.Run("has yaml content", func() {
				s.Nilf(err, "invalid tool result content %v", err)
			})
			s.Run("has 1 item", func() {
				s.Lenf(decoded, 1, "invalid helm list count, expected 1, got %v", len(decoded))
			})
			s.Run("has valid name", func() {
				s.Equalf("release-to-list", decoded[0]["name"], "invalid helm list name, expected release-to-list, got %v", decoded[0]["name"])
			})
			s.Run("has valid status", func() {
				s.Equalf("deployed", decoded[0]["status"], "invalid helm list status, expected deployed, got %v", decoded[0]["status"])
			})
		})
	})
	s.Run("helm_list(namespace=ns-1) with deployed release in other namespaces", func() {
		toolResult, err := s.CallTool("helm_list", map[string]interface{}{"namespace": "ns-1"})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("returns not found", func() {
			s.Equalf("No Helm releases found", toolResult.Content[0].(mcp.TextContent).Text, "unexpected result %v", toolResult.Content[0].(mcp.TextContent).Text)
		})
	})
	s.Run("helm_list(namespace=ns-1, all_namespaces=true) with deployed release in all namespaces", func() {
		toolResult, err := s.CallTool("helm_list", map[string]interface{}{"namespace": "ns-1", "all_namespaces": true})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("returns release", func() {
			var decoded []map[string]interface{}
			err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
			s.Run("has yaml content", func() {
				s.Nilf(err, "invalid tool result content %v", err)
			})
			s.Run("has 1 item", func() {
				s.Lenf(decoded, 1, "invalid helm list count, expected 1, got %v", len(decoded))
			})
			s.Run("has valid name", func() {
				s.Equalf("release-to-list", decoded[0]["name"], "invalid helm list name, expected release-to-list, got %v", decoded[0]["name"])
			})
			s.Run("has valid status", func() {
				s.Equalf("deployed", decoded[0]["status"], "invalid helm list status, expected deployed, got %v", decoded[0]["status"])
			})
		})
	})
}

func (s *HelmSuite) TestHelmListDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { version = "v1", kind = "Secret" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	kc := kubernetes.NewForConfigOrDie(envTestRestConfig)
	_, err := kc.CoreV1().Secrets("default").Create(s.T().Context(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "sh.helm.release.v1.release-to-list-denied",
			Labels: map[string]string{"owner": "helm", "name": "release-to-list-denied"},
		},
		Data: map[string][]byte{
			"release": []byte(base64.StdEncoding.EncodeToString([]byte("{" +
				"\"name\":\"release-to-list-denied\"," +
				"\"info\":{\"status\":\"deployed\"}" +
				"}"))),
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
	s.InitMcpClient()
	s.Run("helm_list() with deployed release (denied)", func() {
		toolResult, err := s.CallTool("helm_list", map[string]interface{}{})
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := toolResult.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			s.Truef(strings.HasPrefix(msg, "failed to list helm releases"), "expected descriptive error, got %v", msg)
			expectedMessage := ": resource not allowed: /v1, Kind=Secret"
			s.Truef(strings.HasSuffix(msg, expectedMessage), "expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
}

func (s *HelmSuite) TestHelmUninstallNoReleases() {
	s.InitMcpClient()
	s.Run("helm_uninstall(name=release-to-uninstall) with no releases", func() {
		toolResult, err := s.CallTool("helm_uninstall", map[string]interface{}{
			"name": "release-to-uninstall",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("returns not found", func() {
			s.Equalf("Release release-to-uninstall not found", toolResult.Content[0].(mcp.TextContent).Text, "unexpected result %v", toolResult.Content[0].(mcp.TextContent).Text)
		})
	})
}

func (s *HelmSuite) TestHelmUninstall() {
	kc := kubernetes.NewForConfigOrDie(envTestRestConfig)
	_, err := kc.CoreV1().Secrets("default").Create(s.T().Context(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "sh.helm.release.v1.existent-release-to-uninstall.v0",
			Labels: map[string]string{"owner": "helm", "name": "existent-release-to-uninstall"},
		},
		Data: map[string][]byte{
			"release": []byte(base64.StdEncoding.EncodeToString([]byte("{" +
				"\"name\":\"existent-release-to-uninstall\"," +
				"\"info\":{\"status\":\"deployed\"}" +
				"}"))),
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
	s.InitMcpClient()
	s.Run("helm_uninstall(name=existent-release-to-uninstall) with deployed release", func() {
		toolResult, err := s.CallTool("helm_uninstall", map[string]interface{}{
			"name": "existent-release-to-uninstall",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("returns uninstalled", func() {
			s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "Uninstalled release existent-release-to-uninstall"), "unexpected result %v", toolResult.Content[0].(mcp.TextContent).Text)
			_, err = kc.CoreV1().Secrets("default").Get(s.T().Context(), "sh.helm.release.v1.existent-release-to-uninstall.v0", metav1.GetOptions{})
			s.Truef(errors.IsNotFound(err), "expected release to be deleted, but it still exists")
		})

	})
}

func (s *HelmSuite) TestHelmUninstallDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { version = "v1", kind = "ConfigMap" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	kc := kubernetes.NewForConfigOrDie(envTestRestConfig)
	_, err := kc.CoreV1().Secrets("default").Create(s.T().Context(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "sh.helm.release.v1.existent-release-to-uninstall.v0",
			Labels: map[string]string{"owner": "helm", "name": "existent-release-to-uninstall"},
		},
		Data: map[string][]byte{
			"release": []byte(base64.StdEncoding.EncodeToString([]byte("{" +
				"\"name\":\"existent-release-to-uninstall\"," +
				"\"info\":{\"status\":\"deployed\"}," +
				"\"manifest\":\"apiVersion: v1\\nkind: ConfigMap\\nmetadata:\\n  name: config-map-to-deny\\n  namespace: default\\n\"" +
				"}"))),
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
	s.InitMcpClient()
	s.Run("helm_uninstall(name=existent-release-to-uninstall) with deployed release (denied)", func() {
		toolResult, err := s.CallTool("helm_uninstall", map[string]interface{}{
			"name": "existent-release-to-uninstall",
		})
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes failure to uninstall", func() {
			s.Contains(toolResult.Content[0].(mcp.TextContent).Text,
				"failed to uninstall helm chart 'existent-release-to-uninstall': failed to delete release: existent-release-to-uninstall")
		})
		s.Run("describes denial (in log)", func() {
			msg := s.logBuffer.String()
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "uninstall: Failed to delete release:(.+:)? resource not allowed: /v1, Kind=ConfigMap"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
}

func clearHelmReleases(ctx context.Context, kc *kubernetes.Clientset) {
	secrets, _ := kc.CoreV1().Secrets("default").List(ctx, metav1.ListOptions{})
	for _, secret := range secrets.Items {
		if strings.HasPrefix(secret.Name, "sh.helm.release.v1.") {
			_ = kc.CoreV1().Secrets("default").Delete(ctx, secret.Name, metav1.DeleteOptions{})
		}
	}
}

func TestHelm(t *testing.T) {
	suite.Run(t, new(HelmSuite))
}
