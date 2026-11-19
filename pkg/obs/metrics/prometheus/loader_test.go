package prometheus

import (
	"context"
	"strings"
	"testing"
)

func TestValidateMetricsExist(t *testing.T) {
	t.Run("Metric exists in Prometheus", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			availableMetrics: []string{"http_requests_total", "node_cpu_seconds_total", "up"},
		}
		loader := &RealLoader{client: mock}

		err := loader.ValidateMetricsExist(context.TODO(), `http_requests_total{job="api"}`)
		if err != nil {
			t.Errorf("expected no error when metric exists, got: %v", err)
		}
	})

	t.Run("Metric does not exist in Prometheus", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			availableMetrics: []string{"http_requests_total", "node_cpu_seconds_total", "up"},
		}
		loader := &RealLoader{client: mock}

		err := loader.ValidateMetricsExist(context.TODO(), `nonexistent_metric{job="api"}`)
		if err == nil {
			t.Error("expected error when metric does not exist")
		}
	})

	t.Run("Multiple metrics all exist", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			availableMetrics: []string{"http_requests_total", "node_cpu_seconds_total", "up"},
		}
		loader := &RealLoader{client: mock}

		err := loader.ValidateMetricsExist(context.TODO(), `http_requests_total{job="api"} + node_cpu_seconds_total{mode="idle"}`)
		if err != nil {
			t.Errorf("expected no error when all metrics exist, got: %v", err)
		}
	})

	t.Run("Multiple metrics - one does not exist", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			availableMetrics: []string{"http_requests_total", "node_cpu_seconds_total", "up"},
		}
		loader := &RealLoader{client: mock}

		err := loader.ValidateMetricsExist(context.TODO(), `http_requests_total{job="api"} + nonexistent_metric{mode="idle"}`)
		if err == nil {
			t.Error("expected error when one metric does not exist")
		}
	})

	t.Run("Metric with __name__ label matcher", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			availableMetrics: []string{"http_requests_total", "node_cpu_seconds_total", "up"},
		}
		loader := &RealLoader{client: mock}

		err := loader.ValidateMetricsExist(context.TODO(), `{__name__="http_requests_total", job="api"}`)
		if err != nil {
			t.Errorf("expected no error when metric exists via __name__ label, got: %v", err)
		}
	})

	t.Run("Complex query with aggregations", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			availableMetrics: []string{"http_requests_total", "node_cpu_seconds_total", "up"},
		}
		loader := &RealLoader{client: mock}

		err := loader.ValidateMetricsExist(context.TODO(), `sum by (job) (rate(http_requests_total{job="api"}[5m]))`)
		if err != nil {
			t.Errorf("expected no error for complex query when metric exists, got: %v", err)
		}
	})

	t.Run("Query with no metrics", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			availableMetrics: []string{"http_requests_total"},
		}
		loader := &RealLoader{client: mock}

		// A scalar value query - no metrics to validate
		err := loader.ValidateMetricsExist(context.TODO(), `1 + 1`)
		if err != nil {
			t.Errorf("expected no error for query with no metrics, got: %v", err)
		}
	})
}

func TestListMetrics_RejectsInvalidRegex(t *testing.T) {
	mock := &mockPrometheusAPI{
		availableMetrics: []string{"up"},
	}
	loader := &RealLoader{client: mock}

	tests := []struct {
		name      string
		regex     string
		wantErr   bool
		errSubstr string
	}{
		{name: "valid regex", regex: "http_requests_.*", wantErr: false},
		{name: "blanket .*", regex: ".*", wantErr: false},
		{name: "blanket .+", regex: ".+", wantErr: false},
		{name: "invalid regex", regex: "[invalid", wantErr: true, errSubstr: "invalid name_regex"},
		{name: "quote injection", regex: `foo"}, other="val`, wantErr: true, errSubstr: "disallowed characters"},
		{name: "brace injection", regex: `foo}`, wantErr: true, errSubstr: "disallowed characters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loader.ListMetrics(context.TODO(), tt.regex)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for regex %q, got nil", tt.regex)
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("expected error containing %q, got: %v", tt.errSubstr, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error for regex %q: %v", tt.regex, err)
			}
		})
	}
}
