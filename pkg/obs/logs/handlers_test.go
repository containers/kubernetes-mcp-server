package logs

import (
	"context"
	"testing"
	"time"

	"github.com/rhobs/obs-mcp/pkg/logs/loki"
)

type mockLoader struct {
	labelNamesFn  func(ctx context.Context, start, end time.Time) ([]string, error)
	labelValuesFn func(ctx context.Context, label string, start, end time.Time) ([]string, error)
	queryRangeFn  func(ctx context.Context, input loki.QueryRangeInput) (loki.QueryRangeResult, error)
}

func (m *mockLoader) LabelNames(ctx context.Context, start, end time.Time) ([]string, error) {
	return m.labelNamesFn(ctx, start, end)
}

func (m *mockLoader) LabelValues(ctx context.Context, label string, start, end time.Time) ([]string, error) {
	return m.labelValuesFn(ctx, label, start, end)
}

func (m *mockLoader) QueryRange(ctx context.Context, input loki.QueryRangeInput) (loki.QueryRangeResult, error) {
	return m.queryRangeFn(ctx, input)
}

func TestLabelValuesHandlerRequiresLabel(t *testing.T) {
	_, err := LabelValuesHandler(ToolParams{
		context:   t.Context(),
		arguments: map[string]any{},
		config:    &Config{LokiURL: "http://localhost:3100"},
		newLokiLoader: func(_, _ string) (loki.Loader, error) {
			return &mockLoader{}, nil
		},
	})
	if err == nil {
		t.Fatalf("expected error when label is missing")
	}
}

func TestQueryRangeHandlerDefaults(t *testing.T) {
	called := false
	output, err := QueryRangeHandler(ToolParams{
		context: t.Context(),
		arguments: map[string]any{
			"query": `{namespace="default"}`,
		},
		config: &Config{LokiURL: "http://localhost:3100"},
		newLokiLoader: func(_, _ string) (loki.Loader, error) {
			return &mockLoader{
				queryRangeFn: func(ctx context.Context, input loki.QueryRangeInput) (loki.QueryRangeResult, error) {
					called = true
					if input.Direction != "backward" {
						t.Fatalf("expected default direction backward, got %s", input.Direction)
					}
					if input.Limit != defaultQueryLimit {
						t.Fatalf("expected default limit %d, got %d", defaultQueryLimit, input.Limit)
					}
					return loki.QueryRangeResult{
						ResultType: "streams",
						Streams: []loki.Stream{
							{Labels: map[string]string{"namespace": "default"}},
						},
					}, nil
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("expected queryRangeFn to be called")
	}
	if output.ResultType != "streams" {
		t.Fatalf("unexpected resultType: %s", output.ResultType)
	}
}
