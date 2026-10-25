package content_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lieranderl/aestheticlab/internal/content"
	"github.com/lieranderl/aestheticlab/internal/i18n"
)

// source is a stand-in Source: fetch answers the nth Fetch (from 1).
type source struct {
	mu      sync.Mutex
	fetches int
	fetch   func(ctx context.Context, n int) (content.Documents, error)
	pingErr error
}

func (s *source) Fetch(ctx context.Context) (content.Documents, error) {
	s.mu.Lock()
	s.fetches++
	n := s.fetches
	s.mu.Unlock()
	return s.fetch(ctx, n)
}

func (s *source) Ping(context.Context) error { return s.pingErr }

func (s *source) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fetches
}

// named is complete content whose first group is called name.
func named(name string) content.Documents {
	d := docs()
	d.ServiceGroups[0]["name"] = name
	return d
}

// answer fetches content whose first group is called name.
func answer(name string) func(context.Context, int) (content.Documents, error) {
	return func(context.Context, int) (content.Documents, error) { return named(name), nil }
}

// clock is a settable clock.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// logs is a log written to from several goroutines.
type logs struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logs) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

type harness struct {
	store *content.Store
	clock *clock
	logs  *logs
}

// newStore is a Store over src with short waits: 50 ms when stale, 200 ms
// when cold.
func newStore(src content.Source) harness {
	h := harness{clock: &clock{now: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)}, logs: &logs{}}
	h.store = content.NewStore(content.Options{
		Source: src, Logger: slog.New(slog.NewJSONHandler(h.logs, nil)), Now: h.clock.Now,
		StaleWait: 50 * time.Millisecond, ColdWait: 200 * time.Millisecond, FetchTimeout: time.Hour,
	})
	return h
}

// firstGroup is the English name of c's first group.
func firstGroup(c *content.Content) string { return c.In(i18n.EnBE).ServiceGroups[0].Name }

func TestAColdRequestRendersTheFetchedContent(t *testing.T) {
	src := &source{fetch: answer("Fetched")}
	h := newStore(src)
	if got := firstGroup(h.store.Content(context.Background())); got != "Fetched" {
		t.Errorf("content = %q, want the fetched content", got)
	}
	h.clock.advance(content.DefaultTTL - time.Second)
	h.store.Content(context.Background())
	if n := src.count(); n != 1 {
		t.Errorf("fetches = %d, want 1 while the content is fresh", n)
	}
}

func TestASlowColdFetchRendersTheSnapshot(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	src := &source{fetch: func(context.Context, int) (content.Documents, error) {
		<-release
		return named("Late"), nil
	}}
	h := newStore(src)
	start := time.Now()
	got := h.store.Content(context.Background())
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waited %v, want about the 200 ms cold wait", elapsed)
	}
	if got != content.Snapshot() {
		t.Errorf("content = %q, want the snapshot", firstGroup(got))
	}
	if !strings.Contains(h.logs.String(), `"msg":"content_snapshot_served"`) {
		t.Errorf("log lacks content_snapshot_served: %s", h.logs)
	}
}

func TestAFailingOrIncompleteSourceFallsBack(t *testing.T) {
	for name, fail := range map[string]func(context.Context, int) (content.Documents, error){
		"error": func(context.Context, int) (content.Documents, error) {
			return content.Documents{}, errors.New("unavailable")
		},
		"incomplete": func(context.Context, int) (content.Documents, error) { return content.Documents{}, nil },
	} {
		t.Run(name, func(t *testing.T) {
			src := &source{fetch: fail}
			h := newStore(src)
			if got := h.store.Content(context.Background()); got != content.Snapshot() {
				t.Errorf("cold content = %q, want the snapshot", firstGroup(got))
			}
			if !strings.Contains(h.logs.String(), `"msg":"content_refresh_failed"`) {
				t.Errorf("log lacks content_refresh_failed: %s", h.logs)
			}

			src.fetch = func(ctx context.Context, n int) (content.Documents, error) {
				if n == 2 {
					return named("Last good"), nil
				}
				return fail(ctx, n)
			}
			first := h.store.Content(context.Background())
			h.clock.advance(content.DefaultTTL)
			if got := h.store.Content(context.Background()); got != first || firstGroup(got) != "Last good" {
				t.Errorf("stale content after a failed refresh = %q, want the last good content", firstGroup(got))
			}
		})
	}
}

func TestConcurrentColdRequestsShareOneFetch(t *testing.T) {
	release := make(chan struct{})
	src := &source{fetch: func(context.Context, int) (content.Documents, error) {
		<-release
		return named("Shared"), nil
	}}
	h := newStore(src)
	h.store.Prefetch()
	var wg sync.WaitGroup
	results := make([]*content.Content, 20)
	for i := range results {
		wg.Go(func() { results[i] = h.store.Content(context.Background()) })
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if n := src.count(); n != 1 {
		t.Errorf("fetches = %d, want 1 shared by the prefetch and every request", n)
	}
	for i, c := range results {
		if firstGroup(c) != "Shared" {
			t.Errorf("request %d got %q, want the shared fetch", i, firstGroup(c))
		}
	}
}

func TestAStaleRequestWaitsBrieflyForTheRefresh(t *testing.T) {
	refresh := make(chan struct{})
	src := &source{fetch: func(_ context.Context, n int) (content.Documents, error) {
		switch n {
		case 1:
			return named("Old"), nil
		case 2:
			return named("Quick"), nil
		}
		<-refresh
		return named("Slow"), nil
	}}
	h := newStore(src)
	h.store.Content(context.Background())

	h.clock.advance(content.DefaultTTL)
	if got := firstGroup(h.store.Content(context.Background())); got != "Quick" {
		t.Errorf("stale request = %q, want the refresh it waited for", got)
	}

	h.clock.advance(content.DefaultTTL)
	start := time.Now()
	if got := firstGroup(h.store.Content(context.Background())); got != "Quick" {
		t.Errorf("stale request with a slow refresh = %q, want the stale content", got)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("waited %v, want about the 50 ms stale wait", elapsed)
	}
	close(refresh)
	deadline := time.Now().Add(2 * time.Second)
	for firstGroup(h.store.Content(context.Background())) != "Slow" {
		if time.Now().After(deadline) {
			t.Fatal("the finished refresh never became current")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAnAbandonedRefreshIsJoinedThenReplaced(t *testing.T) {
	stuck := make(chan struct{})
	defer close(stuck)
	src := &source{fetch: func(_ context.Context, n int) (content.Documents, error) {
		if n == 1 {
			<-stuck // suspended with its instance
			return content.Documents{}, errors.New("timed out")
		}
		return named("Replacement"), nil
	}}
	h := newStore(src)
	h.store.Prefetch()
	h.store.Content(context.Background())
	if n := src.count(); n != 1 {
		t.Fatalf("fetches = %d, want the request to join the refresh under way", n)
	}
	h.clock.advance(2 * time.Hour) // past FetchTimeout
	if got := firstGroup(h.store.Content(context.Background())); got != "Replacement" {
		t.Errorf("content = %q, want a new refresh's", got)
	}
}

func TestAStoreWithoutASourceServesTheSnapshot(t *testing.T) {
	s := content.NewStore(content.Options{})
	if s.Content(context.Background()) != content.Snapshot() {
		t.Error("content is not the snapshot")
	}
	if err := s.Ping(context.Background()); !errors.Is(err, content.ErrNoSource) {
		t.Errorf("Ping() = %v, want ErrNoSource", err)
	}
}

func TestPingAsksTheSource(t *testing.T) {
	src := &source{pingErr: errors.New("unavailable")}
	if err := newStore(src).store.Ping(context.Background()); err == nil {
		t.Error("Ping() = nil, want the source's error")
	}
	src.pingErr = nil
	if err := newStore(src).store.Ping(context.Background()); err != nil {
		t.Errorf("Ping() = %v", err)
	}
	if n := src.count(); n != 0 {
		t.Errorf("Ping fetched %d times, want only the ping", n)
	}
}
