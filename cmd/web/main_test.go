package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer is a log the server writes from other goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// freePort is a TCP port nothing listens on right now.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestTheServerAnswersHealthzThenStopsCleanlyWhenSignalled(t *testing.T) {
	port := freePort(t)
	var logs syncBuffer
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, env(map[string]string{"PORT": strconv.Itoa(port)}), &logs) }()

	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/healthz"
	deadline := time.Now().Add(5 * time.Second)
	for {
		res, err := http.Get(url) //nolint:gosec // the test's own local server
		if err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("/healthz status = %d, want 200", res.StatusCode)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("/healthz never answered: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The embedded static files are served.
	res, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/static/favicon.svg")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/svg+xml" {
		t.Errorf("/static/favicon.svg = %d %q, want 200 image/svg+xml", res.StatusCode, res.Header.Get("Content-Type"))
	}

	stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run = %v, want nil after a shutdown signal", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the server did not stop")
	}

	var messages []string
	for line := range strings.Lines(logs.String()) {
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		// Lifecycle events only: a request's own log line may land
		// after server_stopping.
		if msg := e["message"].(string); strings.HasPrefix(msg, "server_") {
			messages = append(messages, msg)
		}
		if e["message"] == "server_started" && e["port"] != float64(port) {
			t.Errorf("server_started port = %v, want %d", e["port"], port)
		}
	}
	if got := strings.Join(messages, ","); got != "server_started,server_stopping,server_stopped" {
		t.Errorf("lifecycle events = %s", got)
	}
}

func TestThePortDefaultsTo3000(t *testing.T) {
	got, err := listenPort(env(nil))
	if err != nil || got != 3000 {
		t.Fatalf("listenPort = %d, %v; want 3000", got, err)
	}
}

func TestAnInvalidPortStopsTheServerBeforeItStarts(t *testing.T) {
	for _, port := range []string{"web", "0", "70000"} {
		var logs syncBuffer
		if err := run(context.Background(), env(map[string]string{"PORT": port}), &logs); err == nil {
			t.Errorf("PORT=%s: run = nil, want an error", port)
		}
	}
}

func TestWithoutAContentDatabaseTheServerStartsAndSaysSo(t *testing.T) {
	port := freePort(t)
	var logs syncBuffer
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	done := make(chan error, 1)
	go func() { done <- run(ctx, env(map[string]string{"PORT": strconv.Itoa(port)}), &logs) }()

	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/dependencyz"
	deadline := time.Now().Add(5 * time.Second)
	for {
		res, err := http.Get(url) //nolint:gosec // the test's own local server
		if err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			if res.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("/dependencyz status = %d, want 503 without a database", res.StatusCode)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("/dependencyz never answered: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	if err := <-done; err != nil {
		t.Fatalf("run = %v", err)
	}
	if !strings.Contains(logs.String(), `"message":"content_database_not_configured"`) {
		t.Errorf("log lacks content_database_not_configured: %s", logs.String())
	}
}
