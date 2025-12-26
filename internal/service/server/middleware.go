package server

import (
	"crypto/subtle"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// LoggingMiddleware adds request logging
func LoggingMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(rw, r)

			logger.Debug("HTTP request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("remote_addr", r.RemoteAddr),
				zap.Int("status", rw.statusCode),
				zap.Int64("duration_ms", time.Since(start).Milliseconds()))
		})
	}
}

// BasicAuthMiddleware adds HTTP Basic Auth protection
func BasicAuthMiddleware(username, password string, logger *zap.Logger) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			if !ok {
				w.Header().Set("WWW-Authenticate", `Basic realm="Admin Access"`)
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			validUser := subtle.ConstantTimeCompare([]byte(user), []byte(username)) == 1
			validPass := subtle.ConstantTimeCompare([]byte(pass), []byte(password)) == 1

			if !validUser || !validPass {
				w.Header().Set("WWW-Authenticate", `Basic realm="Admin Access"`)
				http.Error(w, "Invalid credentials", http.StatusUnauthorized)
				logger.Warn("failed admin authentication attempt",
					zap.String("username", user),
					zap.String("remote_addr", r.RemoteAddr))
				return
			}

			next(w, r)
		}
	}
}
