// Package usage keeps the chat assistant's model spending in euros and
// stops it at a monthly and a daily cap. Google Cloud budgets only alert,
// and billing data arrives hours late, so this ledger is what actually
// bounds the bill: every reply's tokens are priced (with a margin) and
// recorded before the next message is allowed (plans/ai-salon-assistant.md).
package usage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrDayCap means today's spending reached the daily cap.
	ErrDayCap = errors.New("daily chat budget reached")
	// ErrMonthCap means this month's spending reached the monthly cap.
	ErrMonthCap = errors.New("monthly chat budget reached")
)

// Caps are the spending limits in euros, shared by every environment and
// instance that records into the same store.
type Caps struct {
	MonthEUR, DayEUR float64
}

// Store keeps spending per day and per month.
type Store interface {
	// Spent is what day ("2026-11-22") and month ("2026-11") have spent.
	Spent(ctx context.Context, day, month string) (dayEUR, monthEUR float64, err error)
	// Reserve adds eur to day and month (and env's share) in one atomic
	// step, only if both stay within their caps with it: ErrMonthCap or
	// ErrDayCap otherwise, and nothing is added.
	Reserve(ctx context.Context, day, month, env string, eur float64, caps Caps) error
	// Add adds eur (negative to give back) to day, month and env's share,
	// all or none of them, once.
	Add(ctx context.Context, day, month, env string, eur float64) error
}

// Options configure a Ledger.
type Options struct {
	Store       Store
	Caps        Caps
	Environment string
	Location    *time.Location
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

// Ledger checks and records the chat's spending.
type Ledger struct{ opts Options }

// NewLedger returns a Ledger over opts.
func NewLedger(opts Options) *Ledger {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Ledger{opts: opts}
}

func (l *Ledger) periods() (day, month string) {
	t := l.opts.Now().In(l.opts.Location)
	return t.Format(time.DateOnly), t.Format("2006-01")
}

// Reservation is budget held for one reply until Settle replaces it with
// the reply's real cost.
type Reservation struct {
	day, month string
	eur        float64
}

// Reserve holds eur, the most one reply can cost, before the model is
// called: atomically, and only if today and this month stay within their
// caps with it, so neither a reply nor concurrent replies near a cap can
// spend past it. It returns
// ErrMonthCap or ErrDayCap at a cap, or the store's error, so spending the
// ledger can't see never happens.
func (l *Ledger) Reserve(ctx context.Context, eur float64) (Reservation, error) {
	day, month := l.periods()
	if err := l.opts.Store.Reserve(ctx, day, month, l.opts.Environment, eur, l.opts.Caps); err != nil {
		if errors.Is(err, ErrDayCap) || errors.Is(err, ErrMonthCap) {
			return Reservation{}, err
		}
		return Reservation{}, fmt.Errorf("reserve chat spending: %w", err)
	}
	return Reservation{day: day, month: month, eur: eur}, nil
}

// Settle replaces r with the reply's real cost, in the day and month r was
// taken in.
func (l *Ledger) Settle(ctx context.Context, r Reservation, eur float64) error {
	if delta := eur - r.eur; delta != 0 {
		if err := l.opts.Store.Add(ctx, r.day, r.month, l.opts.Environment, delta); err != nil {
			return fmt.Errorf("record chat spending: %w", err)
		}
	}
	return nil
}

// Allow reports whether today and this month are under their caps, without
// holding anything: for checks, not for admitting a reply (Reserve).
func (l *Ledger) Allow(ctx context.Context) error {
	day, month := l.periods()
	dayEUR, monthEUR, err := l.opts.Store.Spent(ctx, day, month)
	if err != nil {
		return fmt.Errorf("read chat spending: %w", err)
	}
	return l.opts.Caps.check(dayEUR, monthEUR, 0)
}

// tolerance absorbs float sums such as 0.45 + 0.05, which comes to a hair
// over 0.5: a ten-millionth of a cent, far below any reply's cost.
const tolerance = 1e-9

// check is ErrMonthCap or ErrDayCap when adding eur to what the day and
// month spent would pass a cap; with eur 0, once spending reached one.
func (c Caps) check(dayEUR, monthEUR, eur float64) error {
	over := func(spent, limit float64) bool {
		if eur <= 0 {
			return spent+tolerance >= limit
		}
		return spent+eur > limit+tolerance
	}
	switch {
	case over(monthEUR, c.MonthEUR):
		return ErrMonthCap
	case over(dayEUR, c.DayEUR):
		return ErrDayCap
	}
	return nil
}

// Tokens are one reply's billed tokens, summed over its model calls.
type Tokens struct {
	// Prompt is every input token, Cached among them.
	Prompt, Cached int64
	// Output is the reply and the model's thinking, billed alike.
	Output int64
}

// Price is a model's price per million tokens and a safety margin over it.
// Prices are in US dollars and counted as euros, which overstates them.
type Price struct {
	InputPerM, CachedPerM, OutputPerM float64
	Margin                            float64
}

// Cost is t's cost in euros, margin included.
func (p Price) Cost(t Tokens) float64 {
	uncached := max(t.Prompt-t.Cached, 0)
	cost := float64(uncached)*p.InputPerM + float64(t.Cached)*p.CachedPerM + float64(t.Output)*p.OutputPerM
	return cost / 1e6 * p.Margin
}

// margin covers price changes, rounding and Google's own counting.
const margin = 1.25

// prices are Vertex AI's prices at regional endpoints such as eu
// (cloud.google.com/vertex-ai/generative-ai/pricing, 2026-11-22).
var prices = map[string]Price{
	"gemini-3.1-flash-lite": {InputPerM: 0.275, CachedPerM: 0.0275, OutputPerM: 1.65, Margin: margin},
	"gemini-3.5-flash-lite": {InputPerM: 0.33, CachedPerM: 0.033, OutputPerM: 2.75, Margin: margin},
}

// PriceOf is model's price; false for a model this package cannot price,
// whose spending could not be capped.
func PriceOf(model string) (Price, bool) {
	p, ok := prices[model]
	return p, ok
}

// Memory is a Store in this process only: for tests, local development and
// the fake model.
type Memory struct {
	mu    sync.Mutex
	spent map[string]float64
}

// NewMemory returns an empty Memory.
func NewMemory() *Memory { return &Memory{spent: map[string]float64{}} }

// Spent implements Store.
func (m *Memory) Spent(_ context.Context, day, month string) (float64, float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.spent[day], m.spent[month], nil
}

// Reserve implements Store.
func (m *Memory) Reserve(_ context.Context, day, month, _ string, eur float64, caps Caps) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := caps.check(m.spent[day], m.spent[month], eur); err != nil {
		return err
	}
	m.spent[day] += eur
	m.spent[month] += eur
	return nil
}

// Add implements Store.
func (m *Memory) Add(_ context.Context, day, month, _ string, eur float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spent[day] += eur
	m.spent[month] += eur
	return nil
}
