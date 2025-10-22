package http

import (
	"net/http"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

const (
	corsHeaderAllowOrigin      = "Access-Control-Allow-Origin"
	corsHeaderAllowMethods     = "Access-Control-Allow-Methods"
	corsHeaderAllowHeaders     = "Access-Control-Allow-Headers"
	corsHeaderAllowCredentials = "Access-Control-Allow-Credentials"
	corsHeaderExposeHeaders    = "Access-Control-Expose-Headers"
	corsHeaderMaxAge           = "Access-Control-Max-Age"

	corsAllowedMethods = "GET, POST, OPTIONS"
	corsAllowedHeaders = "Authorization, Content-Type, Accept"
	corsExposeHeaders  = "Content-Type, Authorization"
	corsDefaultMaxAge  = 86400
)

func CORSMiddleware(corsConfig *config.CORSConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if corsConfig == nil {
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")
			if origin == "" || !isOriginAllowed(origin, corsConfig.Origins) {
				if r.Method == http.MethodOptions {
					klog.V(2).Infof("CORS preflight request rejected for origin %s", origin)
					w.WriteHeader(http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			setCORSHeaders(w, origin, corsConfig)

			if r.Method == http.MethodOptions {
				maxAge := corsConfig.MaxAge
				if maxAge == 0 {
					maxAge = corsDefaultMaxAge
				}
				w.Header().Set(corsHeaderAllowMethods, corsAllowedMethods)
				w.Header().Set(corsHeaderAllowHeaders, corsAllowedHeaders)
				w.Header().Set(corsHeaderMaxAge, strconv.Itoa(maxAge))

				klog.V(5).Infof("CORS preflight request from origin %s", origin)
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func setCORSHeaders(w http.ResponseWriter, origin string, corsConfig *config.CORSConfig) {
	if len(corsConfig.Origins) == 1 && corsConfig.Origins[0] == "*" {
		w.Header().Set(corsHeaderAllowOrigin, "*")
	} else {
		w.Header().Set(corsHeaderAllowOrigin, origin)
		w.Header().Add("Vary", "Origin")
	}

	// Credentials cannot be used with wildcard origin
	if len(corsConfig.Origins) != 1 || corsConfig.Origins[0] != "*" {
		w.Header().Set(corsHeaderAllowCredentials, "true")
	}

	w.Header().Set(corsHeaderExposeHeaders, corsExposeHeaders)
}

func isOriginAllowed(origin string, allowedOrigins []string) bool {
	if len(allowedOrigins) == 0 {
		return false
	}

	if len(allowedOrigins) == 1 && allowedOrigins[0] == "*" {
		return true
	}

	normalizedOrigin := strings.TrimSuffix(origin, "/")

	for _, allowed := range allowedOrigins {
		normalizedAllowed := strings.TrimSuffix(allowed, "/")
		if normalizedOrigin == normalizedAllowed {
			return true
		}
	}

	return false
}
