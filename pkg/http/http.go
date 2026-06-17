package http

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"k8s.io/klog/v2"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/mcp"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
)

// tlsErrorFilterWriter filters out noisy TLS handshake errors from health checks
type tlsErrorFilterWriter struct {
	underlying io.Writer
}

func (w *tlsErrorFilterWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	// Filter TLS handshake EOF errors - these are typically from
	// load balancer health checks that just do TCP connects.
	// Log at V(4) instead of discarding silently so they can still be seen
	// when debugging with higher verbosity.
	if strings.Contains(msg, "TLS handshake error") && strings.Contains(msg, "EOF") {
		klog.V(4).Infof("TLS handshake error (likely health check): %s", strings.TrimSpace(msg))
		return len(p), nil
	}
	return w.underlying.Write(p)
}

const (
	healthEndpoint     = "/healthz"
	statsEndpoint      = "/stats"
	metricsEndpoint    = "/metrics"
	mcpEndpoint        = "/mcp"
	sseEndpoint        = "/sse"
	sseMessageEndpoint = "/message"
)

var (
	// infraPaths contains infrastructure endpoints which should not have oauth applied
	infraPaths = []string{healthEndpoint, metricsEndpoint, statsEndpoint}
)

// metricsMiddleware wraps an HTTP handler to record metrics for all requests
func metricsMiddleware(next http.Handler, metrics *mcp.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		metrics.GetMetrics().RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, rw.statusCode, duration)
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// statsHandler returns an HTTP handler that exposes server statistics as JSON.
func statsHandler(mcpServer *mcp.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		stats := mcpServer.GetMetrics().GetStats(r.Context())

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(stats); err != nil {
			klogutil.LogInfo(klog.FromContext(r.Context()).V(1), "Failed to encode stats response", klogutil.Err(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}
}

func Serve(ctx context.Context, mcpServer *mcp.Server, cfgState *config.StaticConfigState, oauthState *oauth.State) error {
	logger := klog.FromContext(ctx)
	// Only fields read below are startup-only; middleware reloads via cfgState.
	staticConfig := cfgState.Load()
	mux := http.NewServeMux()

	// Middlewares read config per request from cfgState so SIGHUP reloads
	// take effect immediately. Listed outermost-first (request flow order).
	wrappedMux := chain(mux,
		RequestMiddleware(cfgState),
		AuthorizationMiddleware(cfgState, oauthState),
		MaxBodyMiddleware(cfgState),
	)
	instrumentedHandler := metricsMiddleware(wrappedMux, mcpServer)

	// Note: WriteTimeout is intentionally omitted - it would kill SSE streams.
	// ReadHeaderTimeout provides Slowloris protection; other timeouts are left
	// at Go defaults since MCP clients maintain persistent connections.
	httpServer := &http.Server{
		Addr:              ":" + staticConfig.Port,
		Handler:           instrumentedHandler,
		ReadHeaderTimeout: staticConfig.HTTP.ReadHeaderTimeout.Duration(),
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		// BaseContext propagates the server context (including the klog logger)
		// to all incoming request contexts, so klog.FromContext(r.Context())
		// returns the contextual logger rather than the global fallback.
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}

	// Only set up custom error logger for TLS mode to filter noisy TLS handshake errors
	// from load balancer health checks
	if staticConfig.TLSCert != "" && staticConfig.TLSKey != "" {
		httpServer.ErrorLog = log.New(&tlsErrorFilterWriter{underlying: os.Stderr}, "", 0)
	}

	sseServer := mcpServer.ServeSse()
	streamableHttpServer := mcpServer.ServeHTTP()
	mux.Handle(sseEndpoint, sseServer)
	mux.Handle(sseMessageEndpoint, sseServer)
	mux.Handle(mcpEndpoint, streamableHttpServer)
	mux.HandleFunc(healthEndpoint, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(statsEndpoint, statsHandler(mcpServer))
	mux.Handle(metricsEndpoint, mcpServer.GetMetrics().PrometheusHandler())
	mux.Handle("/.well-known/", WellKnownHandler(cfgState, oauthState))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Drain SIGHUP with a no-op so it never reaches Go's default disposition,
	// which would terminate the process abruptly and bypass graceful shutdown
	// (no httpServer.Shutdown / mcpServer.Shutdown, no metrics flush). In HTTP
	// mode without a config file, cmd/root.go registers no configuration-reload
	// handler, so without this the documented "SIGHUP signals are ignored"
	// behavior would not hold. When a config file IS present, the reload handler
	// in cmd/root.go receives its own copy of the signal (signal.Notify
	// multicasts to every registered channel) and performs the reload; this
	// drain is a harmless no-op in that case.
	sigHupChan := make(chan os.Signal, 1)
	signal.Notify(sigHupChan, syscall.SIGHUP)
	defer signal.Stop(sigHupChan)
	go func() {
		for {
			select {
			case <-sigHupChan:
				// Deliberately neutral wording: when a config is present
				// cmd/root.go's handler performs (and logs) the reload, so Serve
				// must not claim the signal was "ignored" here. All Serve does
				// with SIGHUP is decline to shut down.
				logger.V(5).Info("SIGHUP received by HTTP server; not initiating shutdown")
			case <-ctx.Done():
				return
			}
		}
	}()

	serverErr := make(chan error, 1)
	go func() {
		var err error
		if staticConfig.TLSCert != "" && staticConfig.TLSKey != "" {
			logger.Info("HTTPS server starting",
				"server.port", staticConfig.Port,
				"endpoints", "/mcp, /sse, /message, /healthz, /stats, /metrics",
			)
			err = httpServer.ListenAndServeTLS(staticConfig.TLSCert, staticConfig.TLSKey)
		} else {
			logger.Info("HTTP server starting",
				"server.port", staticConfig.Port,
				"endpoints", "/mcp, /sse, /message, /healthz, /stats, /metrics",
			)
			err = httpServer.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case sig := <-sigChan:
		logger.Info("Received signal, initiating graceful shutdown", "signal", sig.String())
		cancel()
	case <-ctx.Done():
		logger.Info("Context cancelled, initiating graceful shutdown")
	case err := <-serverErr:
		logger.Error(err, "HTTP server error")
		return err
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	logger.Info("Shutting down HTTP server gracefully...")

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		// Don't fail Run() for errors during shutdown
		logger.Error(err, "HTTP server shutdown error")
	}

	// Always attempt MCP server shutdown (flushes metrics) even if HTTP shutdown failed
	if err := mcpServer.Shutdown(shutdownCtx); err != nil {
		// Don't fail Run() for errors during shutdown
		logger.Error(err, "MCP server shutdown error")
	}

	logger.Info("HTTP server shutdown complete")
	return nil
}
