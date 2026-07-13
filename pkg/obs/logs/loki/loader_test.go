package loki

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLabelNames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/labels" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":["namespace","pod"]}`))
	}))
	defer server.Close()

	loader := NewHTTPLoader(server.Client(), server.URL, "")

	labels, err := loader.LabelNames(context.Background(), time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("LabelNames failed: %v", err)
	}
	if len(labels) != 2 || labels[0] != "namespace" || labels[1] != "pod" {
		t.Fatalf("unexpected labels: %v", labels)
	}
}

func TestLabelValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/label/namespace/values" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":["default","kube-system"]}`))
	}))
	defer server.Close()

	loader := NewHTTPLoader(server.Client(), server.URL, "")

	values, err := loader.LabelValues(context.Background(), "namespace", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("LabelValues failed: %v", err)
	}
	if len(values) != 2 || values[0] != "default" || values[1] != "kube-system" {
		t.Fatalf("unexpected values: %v", values)
	}
}

func TestQueryRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/query_range" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
  "status":"success",
  "data":{
    "resultType":"streams",
    "result":[
      {
        "stream":{"namespace":"default","pod":"api-123"},
        "values":[["1710000000000000000","line one"],["1710000001000000000","line two"]]
      }
    ]
  }
}`))
	}))
	defer server.Close()

	loader := NewHTTPLoader(server.Client(), server.URL, "")

	result, err := loader.QueryRange(context.Background(), QueryRangeInput{
		Query:     `{namespace="default"}`,
		Start:     time.Now().Add(-15 * time.Minute),
		End:       time.Now(),
		Limit:     50,
		Direction: "backward",
	})
	if err != nil {
		t.Fatalf("QueryRange failed: %v", err)
	}

	if result.ResultType != "streams" {
		t.Fatalf("unexpected result type: %s", result.ResultType)
	}
	if len(result.Streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(result.Streams))
	}
	if len(result.Streams[0].Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Streams[0].Entries))
	}
}

func TestQueryRangeSetsTenantHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Scope-OrgID"); got != "network" {
			t.Fatalf("unexpected tenant header: %q", got)
		}
		if r.URL.Path != "/loki/api/v1/query_range" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[]}}`))
	}))
	defer server.Close()

	loader := NewHTTPLoader(server.Client(), server.URL, "network")

	_, err := loader.QueryRange(context.Background(), QueryRangeInput{
		Query:     `{job="test"}`,
		Start:     time.Now().Add(-time.Minute),
		End:       time.Now(),
		Limit:     1,
		Direction: "backward",
	})
	if err != nil {
		t.Fatalf("QueryRange failed: %v", err)
	}
}
