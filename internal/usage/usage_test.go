package usage_test

import (
	"context"
	"crypto/rand"
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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

func TestRepliesAreAdmittedUntilTheDailyCap(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, brussels)
	l := ledger(usage.NewMemory(), &now)
	ctx := context.Background()
	for range 4 {
		r, err := l.Reserve(ctx, 0.05)
		if err != nil {
			t.Fatalf("Reserve() = %v before the cap", err)
		}
		if err := l.Settle(ctx, r, 0.125); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.Reserve(ctx, 0.05); !errors.Is(err, usage.ErrDayCap) {
		t.Errorf("Reserve() at €0.50 today = %v, want ErrDayCap", err)
	}
	if err := l.Allow(ctx); !errors.Is(err, usage.ErrDayCap) {
		t.Errorf("Allow() at €0.50 today = %v, want ErrDayCap", err)
	}
	// Midnight in Brussels starts a new day.
	now = time.Date(2026, 11, 23, 0, 0, 1, 0, brussels)
	if _, err := l.Reserve(ctx, 0.05); err != nil {
		t.Errorf("Reserve() the next day = %v", err)
	}
}

func TestTheMonthlyCapHoldsAcrossDays(t *testing.T) {
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, brussels)
	l := ledger(usage.NewMemory(), &now)
	ctx := context.Background()
	for day := range 30 {
		now = time.Date(2026, 11, 1+day, 9, 0, 0, 0, brussels)
		r, err := l.Reserve(ctx, 0.05)
		if err != nil {
			if !errors.Is(err, usage.ErrMonthCap) {
				t.Fatalf("day %d: %v", day+1, err)
			}
			if day+1 != 20 {
				t.Errorf("the month cap stopped on day %d, want day 20 (€9.31 spent after 19 days at €0.49)", day+1)
			}
			break
		}
		_ = l.Settle(ctx, r, 0.49)
	}
	now = time.Date(2026, 12, 1, 9, 0, 0, 0, brussels)
	if _, err := l.Reserve(ctx, 0.05); err != nil {
		t.Errorf("Reserve() in a new month = %v", err)
	}
}

// A reservation is admitted only if it fits under the cap whole: one
// that would pass the cap is refused, even with budget left.
func TestAReservationMustFitUnderTheCap(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		spent float64
		want  error
	}{
		"exact fit":       {spent: 0.45, want: nil},
		"one cent short":  {spent: 0.46, want: usage.ErrDayCap},
		"a cent left":     {spent: 0.49, want: usage.ErrDayCap},
		"plenty left":     {spent: 0.10, want: nil},
		"already reached": {spent: 0.50, want: usage.ErrDayCap},
	} {
		now := time.Date(2026, 11, 22, 10, 0, 0, 0, brussels)
		l := ledger(usage.NewMemory(), &now)
		r, err := l.Reserve(ctx, 0.05)
		if err != nil {
			t.Fatal(err)
		}
		_ = l.Settle(ctx, r, tc.spent)
		if _, err := l.Reserve(ctx, 0.05); !errors.Is(err, tc.want) {
			t.Errorf("%s: Reserve(€0.05) at €%.2f of €0.50 = %v, want %v", name, tc.spent, err, tc.want)
		}
	}
	// The month cap is checked the same way.
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, brussels)
	l := usage.NewLedger(usage.Options{
		Store: usage.NewMemory(), Caps: usage.Caps{MonthEUR: 0.1, DayEUR: 0.5}, Environment: "staging",
		Location: brussels, Now: func() time.Time { return now },
	})
	r, _ := l.Reserve(ctx, 0.05)
	_ = l.Settle(ctx, r, 0.06)
	if _, err := l.Reserve(ctx, 0.05); !errors.Is(err, usage.ErrMonthCap) {
		t.Errorf("Reserve(€0.05) at €0.06 of a €0.10 month = %v, want ErrMonthCap", err)
	}
}

// Near a cap, replies arriving at once can't all pass a check made before
// any of them is recorded: the reservation admits only those that fit,
// which then count.
func TestConcurrentRepliesNearTheCapAdmitOnlyWhatFits(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, brussels)
	store := usage.NewMemory()
	l := ledger(store, &now)
	ctx := context.Background()
	r, _ := l.Reserve(ctx, 0.05)
	_ = l.Settle(ctx, r, 0.40)
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for range 30 {
		wg.Go(func() {
			if _, err := l.Reserve(ctx, 0.05); err == nil {
				admitted.Add(1)
			}
		})
	}
	wg.Wait()
	if got := admitted.Load(); got != 2 {
		t.Errorf("%d of 30 concurrent €0.05 replies admitted at €0.40 of €0.50, want 2", got)
	}
	if day, _, _ := store.Spent(ctx, "2026-11-22", "2026-11"); day > 0.5+1e-9 {
		t.Errorf("€%v held today, past the €0.50 cap", day)
	}
}

// failing is a store that cannot be read.
type failing struct{ usage.Store }

func (failing) Spent(context.Context, string, string) (float64, float64, error) {
	return 0, 0, errors.New("unreachable")
}

func (failing) Reserve(context.Context, string, string, string, float64, usage.Caps) error {
	return errors.New("unreachable")
}

func TestAnUnreadableLedgerStopsTheChat(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, brussels)
	if _, err := ledger(failing{}, &now).Reserve(context.Background(), 0.05); err == nil || errors.Is(err, usage.ErrDayCap) {
		t.Errorf("Reserve() with an unreadable ledger = %v, want a read error", err)
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
	first := ledger(store, &now)
	r, err := first.Reserve(ctx, 0.05)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Settle(ctx, r, 0.3); err != nil {
		t.Fatal(err)
	}
	// Another instance, or this one after a restart, sees the same spending.
	other := usage.NewLedger(usage.Options{
		Store: store, Caps: usage.Caps{MonthEUR: 9, DayEUR: 0.5}, Environment: "production",
		Location: brussels, Now: func() time.Time { return now },
	})
	r, err = other.Reserve(ctx, 0.05)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Settle(ctx, r, 0.25); err != nil {
		t.Fatal(err)
	}
	if _, err := other.Reserve(ctx, 0.05); !errors.Is(err, usage.ErrDayCap) {
		t.Errorf("Reserve() after €0.55 across environments = %v, want ErrDayCap", err)
	}
	// Reservations near the cap are serialized by the transaction.
	now = now.AddDate(0, 0, 1)
	r, _ = other.Reserve(ctx, 0.05)
	_ = other.Settle(ctx, r, 0.40)
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if _, err := other.Reserve(ctx, 0.05); err == nil {
				admitted.Add(1)
			}
		})
	}
	wg.Wait()
	if got := admitted.Load(); got != 2 {
		t.Errorf("%d of 10 concurrent €0.05 reservations admitted at €0.40 of €0.50, want 2", got)
	}
	// Settling moves the day and the month together.
	day := now.Format(time.DateOnly)
	dayEUR, monthEUR, err := store.Spent(ctx, day, now.Format("2006-01"))
	if err != nil {
		t.Fatal(err)
	}
	if want := 0.55 + 0.5; math.Abs(dayEUR-0.5) > 1e-9 || math.Abs(monthEUR-want) > 1e-9 {
		t.Errorf("spent €%v today and €%v this month, want €0.50 and €%v", dayEUR, monthEUR, want)
	}
}
