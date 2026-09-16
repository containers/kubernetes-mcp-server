package guestobservability

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrepareLokiQueryRange(t *testing.T) {
	now := time.Date(
		2026,
		time.August,
		18,
		21,
		0,
		0,
		0,
		time.UTC,
	)

	t.Run("applies safe defaults", func(t *testing.T) {
		values, err := prepareLokiQueryRange(
			LokiQueryRangeRequest{
				Query: `{source="windows_eventlog"}`,
			},
			now,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if values.Get("limit") != "100" {
			t.Fatalf(
				"expected limit 100, got %q",
				values.Get("limit"),
			)
		}

		if values.Get("direction") != "backward" {
			t.Fatalf(
				"expected backward direction, got %q",
				values.Get("direction"),
			)
		}

		start, err := time.Parse(
			time.RFC3339Nano,
			values.Get("start"),
		)
		if err != nil {
			t.Fatalf("invalid start timestamp: %v", err)
		}

		end, err := time.Parse(
			time.RFC3339Nano,
			values.Get("end"),
		)
		if err != nil {
			t.Fatalf("invalid end timestamp: %v", err)
		}

		if !end.Equal(now) {
			t.Fatalf(
				"expected end %v, got %v",
				now,
				end,
			)
		}

		expectedStart := now.Add(-time.Hour)
		if !start.Equal(expectedStart) {
			t.Fatalf(
				"expected start %v, got %v",
				expectedStart,
				start,
			)
		}
	})

	t.Run("normalizes timestamps to UTC", func(t *testing.T) {
		values, err := prepareLokiQueryRange(
			LokiQueryRangeRequest{
				Query: `{source="windows_eventlog"}`,
				Start: "2026-08-18T14:00:00-07:00",
				End:   "2026-08-18T15:00:00-07:00",
			},
			now,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if values.Get("start") !=
			"2026-08-18T21:00:00Z" {
			t.Fatalf(
				"unexpected normalized start: %q",
				values.Get("start"),
			)
		}

		if values.Get("end") !=
			"2026-08-18T22:00:00Z" {
			t.Fatalf(
				"unexpected normalized end: %q",
				values.Get("end"),
			)
		}
	})

	t.Run("supports forward direction and step", func(t *testing.T) {
		values, err := prepareLokiQueryRange(
			LokiQueryRangeRequest{
				Query:     `{source="windows_eventlog"}`,
				Direction: "forward",
				Limit:     50,
				Step:      "30s",
			},
			now,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if values.Get("direction") != "forward" {
			t.Fatalf("unexpected direction")
		}

		if values.Get("limit") != "50" {
			t.Fatalf("unexpected limit")
		}

		if values.Get("step") != "30s" {
			t.Fatalf("unexpected step")
		}
	})

	t.Run("rejects empty query", func(t *testing.T) {
		_, err := prepareLokiQueryRange(
			LokiQueryRangeRequest{},
			now,
		)

		if err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("rejects excessive limit", func(t *testing.T) {
		_, err := prepareLokiQueryRange(
			LokiQueryRangeRequest{
				Query: `{source="windows_eventlog"}`,
				Limit: MaxLokiLimit + 1,
			},
			now,
		)

		if err == nil {
			t.Fatal("expected limit validation error")
		}
	})

	t.Run("rejects invalid direction", func(t *testing.T) {
		_, err := prepareLokiQueryRange(
			LokiQueryRangeRequest{
				Query:     `{source="windows_eventlog"}`,
				Direction: "sideways",
			},
			now,
		)

		if err == nil {
			t.Fatal("expected direction validation error")
		}
	})

	t.Run("rejects reversed time range", func(t *testing.T) {
		_, err := prepareLokiQueryRange(
			LokiQueryRangeRequest{
				Query: `{source="windows_eventlog"}`,
				Start: "2026-08-18T22:00:00Z",
				End:   "2026-08-18T21:00:00Z",
			},
			now,
		)

		if err == nil {
			t.Fatal("expected time-range validation error")
		}
	})
}

func TestLokiQueryRange(t *testing.T) {
	type capturedRequest struct {
		Query  url.Values
		Header http.Header
	}

	captured := make(chan capturedRequest, 1)

	server := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			captured <- capturedRequest{
				Query:  r.URL.Query(),
				Header: r.Header.Clone(),
			}

			w.Header().Set(
				"Content-Type",
				"application/json",
			)

			_ = json.NewEncoder(w).Encode(
				map[string]any{
					"status": "success",
					"data": map[string]any{
						"resultType": "streams",
						"result":     []any{},
					},
				},
			)
		}),
	)
	defer server.Close()

	client, err := newLokiFromConfig(
		&Config{
			Loki: LokiConfig{
				URL:    server.URL,
				Tenant: "application",
			},
		},
		"",
		nil,
		func() bool { return false },
	)
	if err != nil {
		t.Fatalf("failed to create Loki client: %v", err)
	}

	body, err := client.QueryRange(
		context.Background(),
		LokiQueryRangeRequest{
			Query: `{namespace="guest-observability-eval",source="windows_eventlog"} |~ "129"`,
			Start: "2026-08-18T20:00:00Z",
			End:   "2026-08-18T21:00:00Z",
			Limit: 100,
		},
	)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if !strings.Contains(body, `"status":"success"`) &&
		!strings.Contains(body, `"status": "success"`) {
		t.Fatalf("unexpected response body: %s", body)
	}

	request := <-captured

	if request.Query.Get("query") == "" {
		t.Fatal("expected LogQL query")
	}

	if request.Query.Get("start") == "" {
		t.Fatal("expected start timestamp")
	}

	if request.Query.Get("end") == "" {
		t.Fatal("expected end timestamp")
	}

	if request.Query.Get("limit") != "100" {
		t.Fatalf(
			"unexpected limit: %q",
			request.Query.Get("limit"),
		)
	}

	if request.Query.Get("direction") != "backward" {
		t.Fatalf(
			"unexpected direction: %q",
			request.Query.Get("direction"),
		)
	}

	if got := request.Header.Get("Authorization"); got != "" {
		t.Fatalf(
			"unexpected Authorization header sent to Loki: %q",
			got,
		)
	}

	if got := request.Header.Get("X-Scope-OrgID"); got != "application" {
		t.Fatalf(
			"unexpected X-Scope-OrgID: %q",
			got,
		)
	}
}

func TestLokiHTTPError(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			_ *http.Request,
		) {
			http.Error(
				w,
				"backend unavailable",
				http.StatusServiceUnavailable,
			)
		}),
	)
	defer server.Close()

	client, err := newLokiFromConfig(
		&Config{
			Loki: LokiConfig{
				URL: server.URL,
			},
		},
		"",
		nil,
		func() bool { return false },
	)
	if err != nil {
		t.Fatalf("failed to create Loki client: %v", err)
	}

	_, err = client.QueryRange(
		context.Background(),
		LokiQueryRangeRequest{
			Query: `{source="windows_eventlog"}`,
		},
	)

	if err == nil {
		t.Fatal("expected Loki HTTP error")
	}

	if !strings.Contains(
		err.Error(),
		"HTTP 503",
	) {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}
}

func TestLokiRedirectRejected(t *testing.T) {
	target := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			_ *http.Request,
		) {
			w.WriteHeader(http.StatusOK)
		}),
	)
	defer target.Close()

	server := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			http.Redirect(
				w,
				r,
				target.URL,
				http.StatusFound,
			)
		}),
	)
	defer server.Close()

	client, err := newLokiFromConfig(
		&Config{
			Loki: LokiConfig{
				URL: server.URL,
			},
		},
		"",
		nil,
		func() bool { return false },
	)
	if err != nil {
		t.Fatalf("failed to create Loki client: %v", err)
	}

	_, err = client.QueryRange(
		context.Background(),
		LokiQueryRangeRequest{
			Query: `{source="windows_eventlog"}`,
		},
	)
	if err == nil {
		t.Fatal("expected redirect rejection error")
	}

	if !errors.Is(err, errLokiRedirectsNotAllowed) {
		t.Fatalf(
			"expected redirect rejection error, got: %v",
			err,
		)
	}
}

func TestLokiOversizedResponseRejected(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			_ *http.Request,
		) {
			w.Header().Set(
				"Content-Type",
				"application/json",
			)

			_, _ = w.Write(
				[]byte(
					strings.Repeat(
						"x",
						maxLokiResponseBodySize+1,
					),
				),
			)
		}),
	)
	defer server.Close()

	client, err := newLokiFromConfig(
		&Config{
			Loki: LokiConfig{
				URL: server.URL,
			},
		},
		"",
		nil,
		func() bool { return false },
	)
	if err != nil {
		t.Fatalf("failed to create Loki client: %v", err)
	}

	_, err = client.QueryRange(
		context.Background(),
		LokiQueryRangeRequest{
			Query: `{source="windows_eventlog"}`,
		},
	)
	if err == nil {
		t.Fatal("expected oversized Loki response error")
	}

	if !strings.Contains(
		err.Error(),
		"Loki response exceeded maximum size",
	) {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}
}

func TestLokiHTTPClientCached(t *testing.T) {
	client := &Loki{}

	first, err := client.getHTTPClient()
	if err != nil {
		t.Fatalf("failed to create first HTTP client: %v", err)
	}

	second, err := client.getHTTPClient()
	if err != nil {
		t.Fatalf("failed to get cached HTTP client: %v", err)
	}

	if first != second {
		t.Fatal("expected HTTP client to be reused")
	}
}

func TestLokiHTTPClientRebuiltWhenCAFileChanges(t *testing.T) {
	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.crt")

	if err := os.WriteFile(
		caFile,
		createTestCACertificatePEM(t),
		0600,
	); err != nil {
		t.Fatalf("failed to write CA file: %v", err)
	}

	client := &Loki{
		certificateAuthority: caFile,
	}

	first, err := client.getHTTPClient()
	if err != nil {
		t.Fatalf("failed to create first HTTP client: %v", err)
	}

	info, err := os.Stat(caFile)
	if err != nil {
		t.Fatalf("failed to stat CA file: %v", err)
	}

	// Force a modification-time change so the cache observes certificate
	// rotation even on filesystems with coarse timestamp resolution.
	newModTime := info.ModTime().Add(2 * time.Second)

	if err := os.Chtimes(
		caFile,
		newModTime,
		newModTime,
	); err != nil {
		t.Fatalf("failed to update CA file timestamp: %v", err)
	}

	second, err := client.getHTTPClient()
	if err != nil {
		t.Fatalf("failed to rebuild HTTP client: %v", err)
	}

	if first == second {
		t.Fatal("expected HTTP client to be rebuilt after CA file change")
	}
}
