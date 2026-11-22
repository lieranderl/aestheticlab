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
	// Add adds eur to day and month, and to env's share of each.
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

// Allow reports whether the chat may call the model now: ErrMonthCap or
// ErrDayCap once a cap is reached, or the store's error, so that spending
// it cannot check never happens.
func (l *Ledger) Allow(ctx context.Context) error {
	day, month := l.periods()
	dayEUR, monthEUR, err := l.opts.Store.Spent(ctx, day, month)
	switch {
	case err != nil:
		return fmt.Errorf("read chat spending: %w", err)
	case monthEUR >= l.opts.Caps.MonthEUR:
		return ErrMonthCap
	case dayEUR >= l.opts.Caps.DayEUR:
		return ErrDayCap
	}
	return nil
}

// Record adds eur to today's and this month's spending.
func (l *Ledger) Record(ctx context.Context, eur float64) error {
	if eur <= 0 {
		return nil
	}
	day, month := l.periods()
	if err := l.opts.Store.Add(ctx, day, month, l.opts.Environment, eur); err != nil {
		return fmt.Errorf("record chat spending: %w", err)
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

// Add implements Store.
func (m *Memory) Add(_ context.Context, day, month, _ string, eur float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spent[day] += eur
	m.spent[month] += eur
	return nil
}
