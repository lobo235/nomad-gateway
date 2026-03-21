package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// requestLogger returns middleware that logs every request in an access-log style.
func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			log.Info("request",
				"method", r.Method,
				"path", r.URL.RequestURI(),
				"status", rw.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

// bearerAuth returns middleware that enforces Bearer token authentication.
func bearerAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearer(r.Header.Get("Authorization"))
			if subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) != 1 {
				writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid Authorization header")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// extractBearer parses "Bearer <token>" from an Authorization header value.
// Returns an empty string if the header is missing or malformed.
func extractBearer(header string) string {
	prefix := "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}
