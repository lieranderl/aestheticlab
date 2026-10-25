package content

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Source fetches the stored content: Firestore in production, a stand-in
// in tests.
type Source interface {
	// Fetch reads every content document.
	Fetch(ctx context.Context) (Documents, error)
	// Ping reads one document, to show the source answers.
	Ping(ctx context.Context) error
}

// Defaults for Options (plans/go-datastar-scale-to-zero.md, Cold-Start
// Design).
const (
	// DefaultTTL is how long fetched content is served before a request
	// refreshes it: content edits reach visitors within about this long.
	DefaultTTL = 15 * time.Minute
	// DefaultStaleWait is how long a request with stale content waits for
	// the refresh before it renders the stale content.
	DefaultStaleWait = 300 * time.Millisecond
	// DefaultColdWait is how long a request with no content (a cold
	// instance) waits before it renders the snapshot.
	DefaultColdWait = 1500 * time.Millisecond
	// DefaultFetchTimeout bounds a refresh, which outlives the request that
	// started it.
	DefaultFetchTimeout = 5 * time.Second
	// pingTimeout bounds the dependency check's one read.
	pingTimeout = 3 * time.Second
)

// Options configure a Store. Zero values take the defaults.
type Options struct {
	// Source is where content comes from; nil serves the snapshot only.
	Source Source
	Logger *slog.Logger
	// Now is the clock; nil is time.Now.
	Now                                    func() time.Time
	TTL, StaleWait, ColdWait, FetchTimeout time.Duration
}

// ErrNoSource is Ping's error for a Store without a Source.
var ErrNoSource = errors.New("no content database is configured")

// Store is one instance's content cache. Content is fetched inside the
// requests that need it, never on a timer: Cloud Run throttles CPU outside
// requests, so a refresh left running may be suspended and is joined, or
// replaced once its timeout has passed, by the next request.
type Store struct {
	source                                 Source
	log                                    *slog.Logger
	now                                    func() time.Time
	ttl, staleWait, coldWait, fetchTimeout time.Duration

	mu        sync.Mutex
	current   *Content
	fetchedAt time.Time
	flight    *flight
}

// flight is one refresh; done closes when it ends.
type flight struct {
	started time.Time
	done    chan struct{}
	content *Content
	err     error
}

// NewStore returns a Store for opts. It does no I/O.
func NewStore(opts Options) *Store {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Store{
		source:       opts.Source,
		log:          cmp.Or(opts.Logger, slog.Default()),
		now:          now,
		ttl:          cmp.Or(opts.TTL, DefaultTTL),
		staleWait:    cmp.Or(opts.StaleWait, DefaultStaleWait),
		coldWait:     cmp.Or(opts.ColdWait, DefaultColdWait),
		fetchTimeout: cmp.Or(opts.FetchTimeout, DefaultFetchTimeout),
	}
}

// Prefetch starts a refresh without waiting for it. The server calls it
// while the instance starts, when Cloud Run allocates CPU, so the first
// request joins a fetch already under way.
func (s *Store) Prefetch() {
	if s.source != nil {
		s.refresh()
	}
}

// Content is the freshest content available to this request. Fresh
// content is returned at once. Otherwise the request starts (or joins) a
// refresh and waits for it, up to StaleWait when it has stale content and
// ColdWait when it has none, then falls back to the stale content, else the
// embedded snapshot.
func (s *Store) Content(ctx context.Context) *Content {
	if s.source == nil {
		return Snapshot()
	}
	s.mu.Lock()
	current, fresh := s.current, s.current != nil && s.now().Sub(s.fetchedAt) < s.ttl
	s.mu.Unlock()
	if fresh {
		return current
	}
	wait := s.coldWait
	if current != nil {
		wait = s.staleWait
	}
	f := s.refresh()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-f.done:
		if f.err == nil {
			return f.content
		}
	case <-timer.C:
	case <-ctx.Done():
	}
	// Another refresh may have finished meanwhile.
	s.mu.Lock()
	current = s.current
	s.mu.Unlock()
	if current != nil {
		return current
	}
	s.log.WarnContext(ctx, "content_snapshot_served", "wait_ms", wait.Milliseconds())
	return Snapshot()
}

// refresh is the refresh under way, or a new one when there is none or the
// one under way has outlived its timeout (suspended with its instance).
func (s *Store) refresh() *flight {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f := s.flight; f != nil && s.now().Sub(f.started) < s.fetchTimeout {
		return f
	}
	f := &flight{started: s.now(), done: make(chan struct{})}
	s.flight = f
	go s.run(f)
	return f
}

// run fetches and parses the content for f and, when it is complete, makes
// it current.
func (s *Store) run(f *flight) {
	defer close(f.done)
	ctx, cancel := context.WithTimeout(context.Background(), s.fetchTimeout)
	defer cancel()
	start := time.Now()
	docs, err := s.source.Fetch(ctx)
	if err == nil {
		f.content, err = Parse(docs)
	}
	f.err = err
	s.mu.Lock()
	if err == nil && (s.current == nil || !f.started.Before(s.fetchedAt)) {
		s.current, s.fetchedAt = f.content, f.started
	}
	if s.flight == f {
		s.flight = nil
	}
	s.mu.Unlock()
	if err != nil {
		s.log.Error("content_refresh_failed", "error", err.Error(), "duration_ms", time.Since(start).Milliseconds())
		return
	}
	s.log.Info("content_refreshed", "duration_ms", time.Since(start).Milliseconds())
}

// Ping checks that the content source answers, with one bounded read: the
// /dependencyz probe.
func (s *Store) Ping(ctx context.Context) error {
	if s.source == nil {
		return ErrNoSource
	}
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := s.source.Ping(ctx); err != nil {
		return fmt.Errorf("ping content source: %w", err)
	}
	return nil
}
