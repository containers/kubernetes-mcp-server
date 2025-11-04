package kiali

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

func TestMeshStatus_CallsGraphWithExpectedQuery(t *testing.T) {
	var capturedURL *url.URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		_, _ = w.Write([]byte("graph"))
	}))
	defer srv.Close()

	m := NewManager(&config.StaticConfig{KialiOptions: config.KialiOptions{Url: srv.URL}})
	m.BearerToken = "tkn"
	k := m.GetKiali()
	out, err := k.MeshStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "graph" {
		t.Fatalf("unexpected response: %s", out)
	}
	if capturedURL == nil {
		t.Fatalf("expected request to be captured")
	}
	if capturedURL.Path != "/api/mesh/graph" {
		t.Fatalf("unexpected path: %s", capturedURL.Path)
	}
	if capturedURL.Query().Get("includeGateways") != "false" || capturedURL.Query().Get("includeWaypoints") != "false" {
		t.Fatalf("unexpected query: %s", capturedURL.RawQuery)
	}
}
