package usage_test

import (
	"context"
	"crypto/rand"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lieranderl/aestheticlab/internal/usage"
)

var brussels = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Brussels")
	if err != nil {
		panic(err)
	}
	return loc
}()

func ledger(store usage.Store, now *time.Time) *usage.Ledger {
	return usage.NewLedger(usage.Options{
		Store: store, Caps: usage.Caps{MonthEUR: 9, DayEUR: 0.5}, Environment: "staging",
		Location: brussels, Now: func() time.Time { return *now },
	})
}

func TestSpendingIsAllowedUntilTheDailyCap(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, brussels)
	l := ledger(usage.NewMemory(), &now)
	ctx := context.Background()
	for range 4 {
		if err := l.Allow(ctx); err != nil {
			t.Fatalf("Allow() = %v before the cap", err)
		}
		if err := l.Record(ctx, 0.125); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Allow(ctx); !errors.Is(err, usage.ErrDayCap) {
		t.Errorf("Allow() at €0.50 today = %v, want ErrDayCap", err)
	}
	// Midnight in Brussels starts a new day.
	now = time.Date(2026, 11, 23, 0, 0, 1, 0, brussels)
	if err := l.Allow(ctx); err != nil {
		t.Errorf("Allow() the next day = %v", err)
	}
}

func TestTheMonthlyCapHoldsAcrossDays(t *testing.T) {
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, brussels)
	l := ledger(usage.NewMemory(), &now)
	ctx := context.Background()
	for day := range 30 {
		now = time.Date(2026, 11, 1+day, 9, 0, 0, 0, brussels)
		if err := l.Allow(ctx); err != nil {
			if !errors.Is(err, usage.ErrMonthCap) {
				t.Fatalf("day %d: %v", day+1, err)
			}
			if day+1 != 20 {
				t.Errorf("the month cap stopped on day %d, want day 20 (€9.31 spent after 19 days at €0.49)", day+1)
			}
			break
		}
		_ = l.Record(ctx, 0.49)
	}
	now = time.Date(2026, 12, 1, 9, 0, 0, 0, brussels)
	if err := l.Allow(ctx); err != nil {
		t.Errorf("Allow() in a new month = %v", err)
	}
}

// failing is a store that cannot be read.
type failing struct{ usage.Store }

func (failing) Spent(context.Context, string, string) (float64, float64, error) {
	return 0, 0, errors.New("unreachable")
}

func TestAnUnreadableLedgerStopsTheChat(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, brussels)
	if err := ledger(failing{}, &now).Allow(context.Background()); err == nil || errors.Is(err, usage.ErrDayCap) {
		t.Errorf("Allow() with an unreadable ledger = %v, want a read error", err)
	}
}

func TestTokenCostUsesThePriceWithItsMargin(t *testing.T) {
	p := usage.Price{InputPerM: 0.275, CachedPerM: 0.0275, OutputPerM: 1.65, Margin: 1.2}
	got := p.Cost(usage.Tokens{Prompt: 10_000, Cached: 8_000, Output: 500})
	// (2,000 × 0.275 + 8,000 × 0.0275 + 500 × 1.65) / 1e6 × 1.2
	want := (2000*0.275 + 8000*0.0275 + 500*1.65) / 1e6 * 1.2
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("Cost = %v, want %v", got, want)
	}
	if got := p.Cost(usage.Tokens{Prompt: 100, Cached: 500}); got < 0 {
		t.Errorf("more cached than prompt tokens gives a negative cost %v", got)
	}
}

func TestKnownModelsHaveAPrice(t *testing.T) {
	for _, model := range []string{"gemini-3.1-flash-lite", "gemini-3.5-flash-lite"} {
		if p, ok := usage.PriceOf(model); !ok || p.OutputPerM <= 0 || p.Margin < 1 {
			t.Errorf("PriceOf(%q) = %+v, %v", model, p, ok)
		}
	}
	if _, ok := usage.PriceOf("some-new-model"); ok {
		t.Error("an unknown model has a price; spending on it could not be capped")
	}
}

// The Firestore store, against the emulator (make firestore).
func TestFirestoreKeepsSpendingAcrossLedgers(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("Firestore not checked: set FIRESTORE_EMULATOR_HOST (make firestore)")
	}
	ctx := context.Background()
	store, err := usage.NewFirestore(ctx, "demo-aestheticlab", "chat-usage-"+strings.ToLower(rand.Text()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, brussels)
	if err := ledger(store, &now).Record(ctx, 0.3); err != nil {
		t.Fatal(err)
	}
	// Another instance, or this one after a restart, sees the same spending.
	other := usage.NewLedger(usage.Options{
		Store: store, Caps: usage.Caps{MonthEUR: 9, DayEUR: 0.5}, Environment: "production",
		Location: brussels, Now: func() time.Time { return now },
	})
	if err := other.Record(ctx, 0.25); err != nil {
		t.Fatal(err)
	}
	if err := other.Allow(ctx); !errors.Is(err, usage.ErrDayCap) {
		t.Errorf("Allow() after €0.55 across environments = %v, want ErrDayCap", err)
	}
}
