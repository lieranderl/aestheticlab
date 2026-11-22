package server

import (
	"testing"
	"time"
)

func TestAVisitorGetsFortyMessagesADay(t *testing.T) {
	l := newLimiter()
	now := time.Date(2026, 11, 22, 8, 0, 0, 0, time.UTC)
	for i := range visitorDaily {
		if got := l.allow("203.0.113.1", now); got != "" {
			t.Fatalf("message %d refused: %s", i+1, got)
		}
		now = now.Add(visitorEvery) // a steady visitor, never too fast
	}
	if got := l.allow("203.0.113.1", now); got != dailyLimit {
		t.Errorf("message %d = %q, want the daily limit", visitorDaily+1, got)
	}
	if got := l.allow("203.0.113.2", now); got != "" {
		t.Errorf("another visitor was refused: %s", got)
	}
	if got := l.allow("203.0.113.1", time.Date(2026, 11, 23, 0, 0, 1, 0, time.UTC)); got != "" {
		t.Errorf("the next day the visitor was refused: %s", got)
	}
}

func TestABurstIsSlowedDown(t *testing.T) {
	l := newLimiter()
	now := time.Date(2026, 11, 22, 8, 0, 0, 0, time.UTC)
	for range visitorBurst {
		if got := l.allow("203.0.113.9", now); got != "" {
			t.Fatalf("refused within the burst: %s", got)
		}
	}
	if got := l.allow("203.0.113.9", now); got != tooFast {
		t.Errorf("message after the burst = %q, want too fast", got)
	}
}

func TestTheInstanceCapsAllVisitorsTogether(t *testing.T) {
	l := newLimiter()
	now := time.Date(2026, 11, 22, 8, 0, 0, 0, time.UTC)
	refused := 0
	for i := range 2 * instanceBurst {
		if l.allow("198.51.100."+string(rune('a'+i%26))+string(rune('a'+i/26)), now) == tooFast {
			refused++
		}
	}
	if refused != instanceBurst {
		t.Errorf("%d of %d simultaneous visitors refused, want %d", refused, 2*instanceBurst, instanceBurst)
	}
}
