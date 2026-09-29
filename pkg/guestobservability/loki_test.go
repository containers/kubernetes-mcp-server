package guestobservability

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/rest"
)

type LokiSuite struct {
	suite.Suite
}

func TestLokiSuite(t *testing.T) {
	suite.Run(t, new(LokiSuite))
}

func newLokiTestServer(
	t *testing.T,
	handler http.HandlerFunc,
) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return server
}

func (s *LokiSuite) TestPrepareLokiQueryRange() {
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

	s.Run("default behavior", func() {
		s.Run("applies safe defaults", func() {
			values, err := prepareLokiQueryRange(
				LokiQueryRangeRequest{
					Query: `{source="windows_eventlog"}`,
				},
				now,
			)
			s.Require().NoError(err)

			s.Equal("100", values.Get("limit"))
			s.Equal("backward", values.Get("direction"))

			start, err := time.Parse(
				time.RFC3339Nano,
				values.Get("start"),
			)
			s.Require().NoError(
				err,
				"invalid start timestamp",
			)

			end, err := time.Parse(
				time.RFC3339Nano,
				values.Get("end"),
			)
			s.Require().NoError(
				err,
				"invalid end timestamp",
			)

			s.Equal(now, end)
			s.Equal(now.Add(-time.Hour), start)
		})
	})

	s.Run("explicit options", func() {
		s.Run("normalizes timestamps to UTC", func() {
			values, err := prepareLokiQueryRange(
				LokiQueryRangeRequest{
					Query: `{source="windows_eventlog"}`,
					Start: "2026-08-18T14:00:00-07:00",
					End:   "2026-08-18T15:00:00-07:00",
				},
				now,
			)
			s.Require().NoError(err)

			s.Equal(
				"2026-08-18T21:00:00Z",
				values.Get("start"),
			)
			s.Equal(
				"2026-08-18T22:00:00Z",
				values.Get("end"),
			)
		})

		s.Run("supports forward direction and step", func() {
			values, err := prepareLokiQueryRange(
				LokiQueryRangeRequest{
					Query:     `{source="windows_eventlog"}`,
					Direction: "forward",
					Limit:     50,
					Step:      "30s",
				},
				now,
			)
			s.Require().NoError(err)

			s.Equal("forward", values.Get("direction"))
			s.Equal("50", values.Get("limit"))
			s.Equal("30s", values.Get("step"))
		})
	})

	s.Run("invalid requests", func() {
		s.Run("rejects empty query", func() {
			_, err := prepareLokiQueryRange(
				LokiQueryRangeRequest{},
				now,
			)

			s.Error(err)
		})

		s.Run("rejects excessive limit", func() {
			_, err := prepareLokiQueryRange(
				LokiQueryRangeRequest{
					Query: `{source="windows_eventlog"}`,
					Limit: MaxLokiLimit + 1,
				},
				now,
			)

			s.Error(err)
		})

		s.Run("rejects invalid direction", func() {
			_, err := prepareLokiQueryRange(
				LokiQueryRangeRequest{
					Query:     `{source="windows_eventlog"}`,
					Direction: "sideways",
				},
				now,
			)

			s.Error(err)
		})

		s.Run("rejects reversed time range", func() {
			_, err := prepareLokiQueryRange(
				LokiQueryRangeRequest{
					Query: `{source="windows_eventlog"}`,
					Start: "2026-08-18T22:00:00Z",
					End:   "2026-08-18T21:00:00Z",
				},
				now,
			)

			s.Error(err)
		})
	})
}

func (s *LokiSuite) TestNewLokiAuthenticationConfig() {
	cfg, err := config.ReadToml([]byte(`
[toolset_configs.guest-observability.loki]
url = "http://127.0.0.1:3101"
tenant = "application"
`))
	s.Require().NoError(
		err,
		"failed to parse guest-observability configuration",
	)

	s.Run("copies bearer credentials from REST config", func() {
		client, err := NewLoki(
			cfg,
			&rest.Config{
				BearerToken:     " token-xyz ",
				BearerTokenFile: " /var/run/secrets/token ",
			},
		)
		s.Require().NoError(
			err,
			"failed to create Loki client",
		)

		s.Equal(
			"token-xyz",
			client.bearerToken,
			"unexpected bearer token",
		)
		s.Equal(
			"/var/run/secrets/token",
			client.bearerTokenFile,
			"unexpected bearer token file",
		)
	})

	s.Run("supports unauthenticated Loki", func() {
		client, err := NewLoki(
			cfg,
			nil,
		)
		s.Require().NoError(
			err,
			"failed to create unauthenticated Loki client",
		)

		s.Empty(
			client.bearerToken,
			"did not expect bearer token",
		)
		s.Empty(
			client.bearerTokenFile,
			"did not expect bearer token file",
		)
	})
}

func (s *LokiSuite) TestLokiQueryRange() {
	type capturedRequest struct {
		Query  url.Values
		Header http.Header
	}

	captured := make(chan capturedRequest, 1)

	server := newLokiTestServer(
		s.T(),
		func(
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
		},
	)

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
	s.Require().NoError(
		err,
		"failed to create Loki client",
	)

	body, err := client.QueryRange(
		context.Background(),
		LokiQueryRangeRequest{
			Query: `{namespace="guest-observability-eval",source="windows_eventlog"} |~ "129"`,
			Start: "2026-08-18T20:00:00Z",
			End:   "2026-08-18T21:00:00Z",
			Limit: 100,
		},
	)
	s.Require().NoError(err, "query failed")

	s.True(
		strings.Contains(body, `"status":"success"`) ||
			strings.Contains(body, `"status": "success"`),
		"unexpected response body: %s",
		body,
	)

	request := <-captured

	s.NotEmpty(
		request.Query.Get("query"),
		"expected LogQL query",
	)
	s.NotEmpty(
		request.Query.Get("start"),
		"expected start timestamp",
	)
	s.NotEmpty(
		request.Query.Get("end"),
		"expected end timestamp",
	)
	s.Equal(
		"100",
		request.Query.Get("limit"),
		"unexpected limit",
	)
	s.Equal(
		"backward",
		request.Query.Get("direction"),
		"unexpected direction",
	)
	s.Empty(
		request.Header.Get("Authorization"),
		"unexpected Authorization header sent to Loki",
	)
	s.Equal(
		"application",
		request.Header.Get("X-Scope-OrgID"),
		"unexpected X-Scope-OrgID",
	)
}

func (s *LokiSuite) TestLokiAuthorization() {
	ctx := context.Background()
	query := LokiQueryRangeRequest{
		Query: `{source="windows_eventlog"}`,
	}

	s.Run("sets bearer authorization header", func() {
		client := &Loki{
			baseURL:     "http://example.com",
			bearerToken: "token-xyz",
		}

		req, err := client.newQueryRangeRequest(
			ctx,
			query,
		)
		s.Require().NoError(err)

		s.Equal(
			"Bearer token-xyz",
			req.Header.Get("Authorization"),
			"unexpected Authorization header",
		)
	})

	s.Run("does not duplicate bearer prefix", func() {
		client := &Loki{
			baseURL:     "http://example.com",
			bearerToken: "Bearer token-xyz",
		}

		req, err := client.newQueryRangeRequest(
			ctx,
			query,
		)
		s.Require().NoError(err)

		s.Equal(
			"Bearer token-xyz",
			req.Header.Get("Authorization"),
			"unexpected Authorization header",
		)
	})

	s.Run("reads bearer token file at request time", func() {
		tokenFile := filepath.Join(
			s.T().TempDir(),
			"token",
		)

		s.Require().NoError(
			os.WriteFile(
				tokenFile,
				[]byte("token-one\n"),
				0600,
			),
		)

		client := &Loki{
			baseURL:         "http://example.com",
			bearerTokenFile: tokenFile,
		}

		first, err := client.newQueryRangeRequest(
			ctx,
			query,
		)
		s.Require().NoError(err)

		s.Equal(
			"Bearer token-one",
			first.Header.Get("Authorization"),
			"unexpected Authorization header",
		)

		s.Require().NoError(
			os.WriteFile(
				tokenFile,
				[]byte("token-two\n"),
				0600,
			),
		)

		second, err := client.newQueryRangeRequest(
			ctx,
			query,
		)
		s.Require().NoError(err)

		s.Equal(
			"Bearer token-two",
			second.Header.Get("Authorization"),
			"expected rotated bearer token to be read from file",
		)
	})

	s.Run("prefers inline bearer token over token file", func() {
		tokenFile := filepath.Join(
			s.T().TempDir(),
			"token",
		)

		s.Require().NoError(
			os.WriteFile(
				tokenFile,
				[]byte("file-token\n"),
				0600,
			),
		)

		client := &Loki{
			baseURL:         "http://example.com",
			bearerToken:     "inline-token",
			bearerTokenFile: tokenFile,
		}

		req, err := client.newQueryRangeRequest(
			ctx,
			query,
		)
		s.Require().NoError(err)

		s.Equal(
			"Bearer inline-token",
			req.Header.Get("Authorization"),
			"expected inline bearer token to take precedence",
		)
	})

	s.Run("returns error when configured bearer token file cannot be read", func() {
		missingTokenFile := filepath.Join(
			s.T().TempDir(),
			"missing-token",
		)

		client := &Loki{
			baseURL:         "http://example.com",
			bearerTokenFile: missingTokenFile,
		}

		req, err := client.newQueryRangeRequest(
			ctx,
			query,
		)

		s.Require().Error(
			err,
			"expected unreadable bearer token file to fail request creation",
		)
		s.Nil(
			req,
			"did not expect request when bearer token file cannot be read",
		)
		s.Contains(
			err.Error(),
			"failed to read Loki bearer token file",
			"unexpected error",
		)
		s.Contains(
			err.Error(),
			missingTokenFile,
			"expected token file path in error",
		)
	})

	s.Run("omits authorization when credentials are absent", func() {
		client := &Loki{
			baseURL: "http://example.com",
		}

		req, err := client.newQueryRangeRequest(
			ctx,
			query,
		)
		s.Require().NoError(err)

		s.Empty(
			req.Header.Get("Authorization"),
			"expected Authorization header to be omitted",
		)
	})
}

func (s *LokiSuite) TestLokiHTTPError() {
	server := newLokiTestServer(
		s.T(),
		func(
			w http.ResponseWriter,
			_ *http.Request,
		) {
			http.Error(
				w,
				"backend unavailable",
				http.StatusServiceUnavailable,
			)
		},
	)

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
	s.Require().NoError(
		err,
		"failed to create Loki client",
	)

	_, err = client.QueryRange(
		context.Background(),
		LokiQueryRangeRequest{
			Query: `{source="windows_eventlog"}`,
		},
	)

	s.Require().Error(err, "expected Loki HTTP error")
	s.Contains(err.Error(), "HTTP 503")
}

func (s *LokiSuite) TestLokiRedirectRejected() {
	target := newLokiTestServer(
		s.T(),
		func(
			w http.ResponseWriter,
			_ *http.Request,
		) {
			w.WriteHeader(http.StatusOK)
		},
	)

	server := newLokiTestServer(
		s.T(),
		func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			http.Redirect(
				w,
				r,
				target.URL,
				http.StatusFound,
			)
		},
	)

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
	s.Require().NoError(
		err,
		"failed to create Loki client",
	)

	_, err = client.QueryRange(
		context.Background(),
		LokiQueryRangeRequest{
			Query: `{source="windows_eventlog"}`,
		},
	)

	s.Require().Error(
		err,
		"expected redirect rejection error",
	)
	s.ErrorIs(err, errLokiRedirectsNotAllowed)
}

func (s *LokiSuite) TestLokiOversizedResponseRejected() {
	server := newLokiTestServer(
		s.T(),
		func(
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
		},
	)

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
	s.Require().NoError(
		err,
		"failed to create Loki client",
	)

	_, err = client.QueryRange(
		context.Background(),
		LokiQueryRangeRequest{
			Query: `{source="windows_eventlog"}`,
		},
	)

	s.Require().Error(
		err,
		"expected oversized Loki response error",
	)
	s.Contains(
		err.Error(),
		"Loki response exceeded maximum size",
	)
}

func resetLokiTransportCache() {
	lokiTransportCache.Lock()
	defer lokiTransportCache.Unlock()

	if lokiTransportCache.entry != nil {
		lokiTransportCache.entry.transport.CloseIdleConnections()
	}

	lokiTransportCache.entry = nil
}

func (s *LokiSuite) TestLokiHTTPClient() {
	s.T().Cleanup(resetLokiTransportCache)

	s.Run("reuses transport across Loki instances", func() {
		resetLokiTransportCache()
		firstLoki := &Loki{}
		secondLoki := &Loki{}

		first, err := firstLoki.getHTTPClient()
		s.Require().NoError(
			err,
			"failed to create first HTTP client",
		)

		second, err := secondLoki.getHTTPClient()
		s.Require().NoError(
			err,
			"failed to create second HTTP client",
		)

		s.NotSame(
			first,
			second,
			"HTTP clients may be created per Loki instance",
		)

		s.Same(
			first.Transport,
			second.Transport,
			"expected HTTP transport to be reused across Loki instances",
		)
	})

	s.Run("rebuilds transport when CA file changes", func() {
		resetLokiTransportCache()
		dir := s.T().TempDir()
		caFile := filepath.Join(dir, "ca.crt")

		s.Require().NoError(
			os.WriteFile(
				caFile,
				createTestCACertificatePEM(s.T()),
				0600,
			),
			"failed to write CA file",
		)

		firstLoki := &Loki{
			certificateAuthority: caFile,
		}

		first, err := firstLoki.getHTTPClient()
		s.Require().NoError(
			err,
			"failed to create first HTTP client",
		)

		info, err := os.Stat(caFile)
		s.Require().NoError(
			err,
			"failed to stat CA file",
		)

		// Force a modification-time change so the cache observes certificate
		// rotation even on filesystems with coarse timestamp resolution.
		newModTime := info.ModTime().Add(2 * time.Second)

		s.Require().NoError(
			os.Chtimes(
				caFile,
				newModTime,
				newModTime,
			),
			"failed to update CA file timestamp",
		)

		secondLoki := &Loki{
			certificateAuthority: caFile,
		}

		second, err := secondLoki.getHTTPClient()
		s.Require().NoError(
			err,
			"failed to rebuild HTTP client",
		)

		s.NotSame(
			first.Transport,
			second.Transport,
			"expected HTTP transport to be rebuilt after CA file change",
		)
	})

	s.Run("does not share transport across TLS configurations", func() {
		resetLokiTransportCache()
		firstLoki := &Loki{
			insecure: false,
		}
		secondLoki := &Loki{
			insecure: true,
		}

		first, err := firstLoki.getHTTPClient()
		s.Require().NoError(
			err,
			"failed to create first HTTP client",
		)

		second, err := secondLoki.getHTTPClient()
		s.Require().NoError(
			err,
			"failed to create second HTTP client",
		)

		s.NotSame(
			first.Transport,
			second.Transport,
			"expected different TLS configurations to use different transports",
		)
	})
}
