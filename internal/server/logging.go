package server

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// NewLogger writes JSON lines that Cloud Logging reads as structured logs:
// severity (DEBUG, INFO, WARNING, ERROR) and message, as
// src/shared/server-logging.ts writes them. Messages are snake_case event
// names, such as server_started.
func NewLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) > 0 {
				return a
			}
			switch a.Key {
			case slog.LevelKey:
				return slog.String("severity", severity(a.Value.Any().(slog.Level)))
			case slog.MessageKey:
				a.Key = "message"
			}
			return a
		},
	}))
}

// severity is Cloud Logging's name for level.
func severity(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "ERROR"
	case level >= slog.LevelWarn:
		return "WARNING"
	case level >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}

// Middleware wraps a handler in the site's security headers, request log,
// page compression and panic recovery.
func Middleware(log *slog.Logger, next http.Handler) http.Handler {
	return securityHeaders(logRequests(log, gzipPages(recoverPanics(log, next))))
}

// logRequests logs each request as http_request, an error when it failed
// (5xx). The query string is left out, as it may hold what a visitor typed.
// Successful probes are not logged: Cloud Run's startup probe hits /healthz
// every second while an instance starts.
func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if isProbe(r.URL.Path) && rec.status == http.StatusOK {
			return
		}
		level := slog.LevelInfo
		if rec.status >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		log.LogAttrs(r.Context(), level, "http_request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
		)
	})
}

func isProbe(path string) bool {
	return path == "/healthz" || path == "/readyz" || path == "/dependencyz"
}

// recoverPanics logs http_panic and answers 500 before headers are sent.
// After headers are sent it aborts the transfer: a partial page must never
// finish as a successful response.
func recoverPanics(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w}
		defer func() {
			if v := recover(); v != nil {
				// net/http's own way to abort a response stays a panic.
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v)
				}
				log.ErrorContext(r.Context(), "http_panic",
					"method", r.Method, "path", r.URL.Path, "panic", fmt.Sprint(v))
				if rec.wroteHeader {
					panic(http.ErrAbortHandler)
				}
				http.Error(rec, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(rec, r)
	})
}

// statusRecorder notes the status a handler wrote. Flush and Unwrap keep
// streaming (Datastar's server-sent events) and http.ResponseController
// working through it.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rec *statusRecorder) WriteHeader(status int) {
	if !rec.wroteHeader {
		rec.status = status
		rec.wroteHeader = true
	}
	rec.ResponseWriter.WriteHeader(status)
}

func (rec *statusRecorder) Write(b []byte) (int, error) {
	if !rec.wroteHeader {
		rec.WriteHeader(http.StatusOK)
	}
	return rec.ResponseWriter.Write(b)
}

func (rec *statusRecorder) Flush() {
	if !rec.wroteHeader {
		rec.WriteHeader(http.StatusOK)
	}
	if f, ok := rec.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (rec *statusRecorder) Unwrap() http.ResponseWriter { return rec.ResponseWriter }
