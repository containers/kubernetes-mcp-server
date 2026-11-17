//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

const (
	runbookE2EAlertName = "MCPRunbookE2E"
	runbookE2EURL       = "https://kubernetes-mcp-server.test/runbook-e2e"
	runbookE2EPromRule  = "test/e2e/manifests/obs/prometheus/runbook_alert_prometheusrule.yaml"
)

type obsRunbookState struct {
	dep       *serverDeployment
	mcpClient *test.McpClient
}

var obsRunbookTS testState[obsRunbookState]

func TestObsMetricsRunbookURL(t *testing.T) {
	f := features.New("obs-metrics-runbook").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			clientset, err := clientsetFromKubeconfig(cfg.KubeconfigFile())
			require.NoError(t, err)
			_, err = clientset.CoreV1().Namespaces().Get(ctx, "monitoring", metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				t.Skip("monitoring namespace missing — run make e2e-setup-obs")
			}
			require.NoError(t, err, "get monitoring namespace")

			// Rule must carry prometheus=k8s and role=alert-rules so kube-prometheus loads it.
			manifest := repoFilePath(t, runbookE2EPromRule)
			require.NoError(t, kubectlApply(cfg.KubeconfigFile(), manifest))
			t.Cleanup(func() {
				_ = kubectlDelete(cfg.KubeconfigFile(), manifest)
			})

			dep := deployServer(ctx, t, cfg, "obs-runbook",
				withConfig(obsMetricsConfig()),
				withValues(viewClusterRoleBindingValues()),
			)
			mcpClient := test.NewMcpClient(t, nil, test.WithEndpoint(dep.serverURL+"/mcp"))
			t.Cleanup(mcpClient.Close)
			return obsRunbookTS.set(ctx, &obsRunbookState{dep: dep, mcpClient: mcpClient})
		}).
		Assess("get_alerts promotes runbook_url from annotations", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsRunbookTS.get(ctx)
			var output string
			deadline := time.Now().Add(3 * time.Minute)
			for {
				output = requireObsToolCallSuccess(t, s.mcpClient, "get_alerts", map[string]any{
					"filter": "alertname=" + runbookE2EAlertName,
				})
				if alertHasRunbookURL(t, output, runbookE2EURL) {
					return ctx
				}
				if time.Now().After(deadline) {
					break
				}
				time.Sleep(10 * time.Second)
			}
			require.Fail(t, "expected get_alerts to return "+runbookE2EAlertName+" with runbook_url "+runbookE2EURL, "last output: %s", output)
			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}

func repoFilePath(t *testing.T, repoRelative string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	return filepath.Join(repoRoot, filepath.FromSlash(repoRelative))
}

func kubectlBin() string {
	return envOrDefault("KUBECTL_PATH", "kubectl")
}

func kubectlApply(kubeconfig, manifest string) error {
	cmd := exec.Command(kubectlBin(), "--kubeconfig", kubeconfig, "apply", "-f", manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return execError("apply", manifest, out, err)
	}
	return nil
}

func kubectlDelete(kubeconfig, manifest string) error {
	cmd := exec.Command(kubectlBin(), "--kubeconfig", kubeconfig, "delete", "-f", manifest, "--ignore-not-found")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return execError("delete", manifest, out, err)
	}
	return nil
}

func execError(op, manifest string, out []byte, err error) error {
	return &kubectlError{op: op, manifest: manifest, out: string(out), err: err}
}

type kubectlError struct {
	op, manifest, out string
	err               error
}

func (e *kubectlError) Error() string {
	return e.op + " " + e.manifest + ": " + e.err.Error() + "\n" + e.out
}

func alertHasRunbookURL(t *testing.T, getAlertsJSON, wantURL string) bool {
	t.Helper()
	var payload struct {
		Alerts []struct {
			RunbookURL string            `json:"runbook_url"`
			Labels     map[string]string `json:"labels"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal([]byte(getAlertsJSON), &payload); err != nil {
		return false
	}
	for _, a := range payload.Alerts {
		if a.Labels["alertname"] != runbookE2EAlertName {
			continue
		}
		if a.RunbookURL == wantURL {
			return true
		}
	}
	return false
}
