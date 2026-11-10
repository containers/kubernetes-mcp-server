package metrics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"k8s.io/client-go/rest"
)

func TestPrometheusAndAlertmanagerCapBackendResponse(t *testing.T) {
	const promBody = `{"status":"success","data":["up"]}`
	alertBody := `["` + strings.Repeat("a", len(promBody)) + `"]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := promBody
		if r.URL.Path == "/api/v2/alerts" {
			body = alertBody
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	serverCfg := &config.Config{}
	serverCfg.MaxBackendResponseBytes.SetForTest(int64(len(promBody) - 1))

	t.Run("prometheus", func(t *testing.T) {
		params := newTestParams(context.Background(), &rest.Config{}, &Config{PrometheusURL: srv.URL})
		params.Config = serverCfg
		client, err := getPromClient(params)
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		_, err = client.ListMetrics(context.Background(), ".*")
		var tooLarge *kubernetes.BackendResponseTooLargeError
		if !errors.As(err, &tooLarge) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("alertmanager", func(t *testing.T) {
		params := newTestParams(context.Background(), &rest.Config{}, &Config{AlertmanagerURL: srv.URL})
		params.Config = serverCfg
		client, err := getAlertmanagerClient(params)
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		_, err = client.GetAlerts(context.Background(), nil, nil, nil, nil, nil, "")
		var tooLarge *kubernetes.BackendResponseTooLargeError
		if !errors.As(err, &tooLarge) {
			t.Fatalf("got %v", err)
		}
	})
}
