package server_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lieranderl/aestheticlab/internal/server"
)

// entries are the JSON log lines written to buf.
func entries(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.Lines(buf.String()) {
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		out = append(out, e)
	}
	return out
}

// Cloud Logging reads severity and message from these fields, as it does for
// src/shared/server-logging.ts.
func TestLogsUseCloudLoggingFields(t *testing.T) {
	var buf bytes.Buffer
	log := server.NewLogger(&buf)
	log.Info("server_started", "port", 3000)
	log.Warn("something_odd")
	log.Error("something_failed")

	got := entries(t, &buf)
	if len(got) != 3 {
		t.Fatalf("%d entries, want 3", len(got))
	}
	if got[0]["severity"] != "INFO" || got[0]["message"] != "server_started" || got[0]["port"] != float64(3000) {
		t.Errorf("entry = %v", got[0])
	}
	if got[1]["severity"] != "WARNING" {
		t.Errorf("warn severity = %v, want WARNING", got[1]["severity"])
	}
	if got[2]["severity"] != "ERROR" {
		t.Errorf("error severity = %v, want ERROR", got[2]["severity"])
	}
	for _, e := range got {
		if _, ok := e["level"]; ok {
			t.Errorf("entry has slog's level field: %v", e)
		}
	}
}

func TestEachRequestIsLoggedWithoutItsQuery(t *testing.T) {
	var buf bytes.Buffer
	h := server.Middleware(server.NewLogger(&buf), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/en-BE/?email=someone@example.com", nil))

	got := entries(t, &buf)
	if len(got) != 1 {
		t.Fatalf("%d entries, want 1: %s", len(got), buf.String())
	}
	e := got[0]
	if e["message"] != "http_request" || e["severity"] != "INFO" || e["method"] != "GET" || e["path"] != "/en-BE/" || e["status"] != float64(418) {
		t.Errorf("entry = %v", e)
	}
	if _, ok := e["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms = %v, want a number", e["duration_ms"])
	}
	if strings.Contains(buf.String(), "example.com") {
		t.Errorf("the query string was logged: %s", buf.String())
	}
}

func TestSuccessfulProbesAreNotLogged(t *testing.T) {
	var buf bytes.Buffer
	h := server.New(server.Config{Logger: server.NewLogger(&buf)})
	for _, path := range []string{"/healthz", "/readyz", "/dependencyz"} {
		get(t, h, path)
	}
	if buf.Len() != 0 {
		t.Errorf("probes were logged: %s", buf.String())
	}
}

func TestAPanicAnswers500AndIsLoggedAsAnError(t *testing.T) {
	var buf bytes.Buffer
	h := server.Middleware(server.NewLogger(&buf), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/en-BE/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("the 500 lost the security headers")
	}
	var messages []string
	for _, e := range entries(t, &buf) {
		if e["severity"] != "ERROR" {
			t.Errorf("severity = %v, want ERROR: %v", e["severity"], e)
		}
		messages = append(messages, e["message"].(string))
	}
	if strings.Join(messages, ",") != "http_panic,http_request" {
		t.Errorf("messages = %v, want http_panic then http_request", messages)
	}
}

func TestAPanicAfterFlushingAbortsTheResponse(t *testing.T) {
	var logs bytes.Buffer
	h := server.Middleware(server.NewLogger(&logs), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "partial page")
		w.(http.Flusher).Flush()
		panic("render failed")
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()
	client := srv.Client()
	client.Timeout = 5 * time.Second
	res, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("partial response read = %q, %v; want an aborted transfer", body, err)
	}
	// Close waits for the handler, so reading its log is race-free.
	srv.Close()
	got := entries(t, &logs)
	if len(got) != 1 || got[0]["message"] != "http_panic" || got[0]["severity"] != "ERROR" {
		t.Errorf("panic log = %v; want one http_panic error", got)
	}
}
