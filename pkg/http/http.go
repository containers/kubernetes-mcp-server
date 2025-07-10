package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/klog/v2"

	"github.com/manusa/kubernetes-mcp-server/pkg/config"
	"github.com/manusa/kubernetes-mcp-server/pkg/mcp"
)

func Serve(ctx context.Context, mcpServer *mcp.Server, staticConfig *config.StaticConfig, sseBaseUrl string) error {
	mux := http.NewServeMux()

	wrappedMux := RequestMiddleware(
		AuthorizationMiddleware(staticConfig.RequireOAuth, mcpServer)(mux),
	)

	httpServer := &http.Server{
		Addr:    ":" + staticConfig.Port,
		Handler: wrappedMux,
	}

	sseServer := mcpServer.ServeSse(sseBaseUrl, httpServer)
	streamableHttpServer := mcpServer.ServeHTTP(httpServer)
	mux.Handle("/sse", sseServer)
	mux.Handle("/message", sseServer)
	mux.Handle("/mcp", streamableHttpServer)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var authServers []string
		if staticConfig.AuthorizationServer != "" {
			authServers = []string{staticConfig.AuthorizationServer}
		} else {
			// Fallback to Kubernetes API server host if authorization_server is not configured
			if apiServerHost := mcpServer.GetKubernetesAPIServerHost(); apiServerHost != "" {
				authServers = []string{apiServerHost}
			}
		}

		response := map[string]interface{}{
			"authorization_servers": authServers,
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		klog.V(0).Infof("Streaming and SSE HTTP servers starting on port %s and paths /mcp, /sse, /message", staticConfig.Port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case sig := <-sigChan:
		klog.V(0).Infof("Received signal %v, initiating graceful shutdown", sig)
		cancel()
	case <-ctx.Done():
		klog.V(0).Infof("Context cancelled, initiating graceful shutdown")
	case err := <-serverErr:
		klog.Errorf("HTTP server error: %v", err)
		return err
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	klog.V(0).Infof("Shutting down HTTP server gracefully...")
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		klog.Errorf("HTTP server shutdown error: %v", err)
		return err
	}

	klog.V(0).Infof("HTTP server shutdown complete")
	return nil
}
