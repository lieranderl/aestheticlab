package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/starfederation/datastar-go/datastar"
	"golang.org/x/time/rate"

	"github.com/lieranderl/aestheticlab/internal/assistant"
	"github.com/lieranderl/aestheticlab/internal/content"
	"github.com/lieranderl/aestheticlab/internal/i18n"
	"github.com/lieranderl/aestheticlab/internal/usage"
	"github.com/lieranderl/aestheticlab/internal/views"
)

const (
	// maxChatBody bounds a chat request: the input and the transcript.
	maxChatBody = 64 << 10
	// maxMessage is the longest visitor message, in characters (the input's
	// maxlength).
	maxMessage = 500
	// maxTurns is how much of the transcript the assistant sees.
	maxTurns = 20
	// maxTurnText bounds one transcript message, in characters.
	maxTurnText = 2000
	// replyTimeout bounds one answer from the budget's reservation on, tool
	// calls included.
	replyTimeout = 30 * time.Second
)

// chatSignals are the signals a chat request carries (views.chatSignals).
type chatSignals struct {
	ChatInput string `json:"chatInput"`
	// ChatLookup is the lookup a choice's button asks for (encodeLookup):
	// its free times are looked up directly, never by the model.
	ChatLookup  string           `json:"chatLookup"`
	ChatHistory []assistant.Turn `json:"chatHistory"`
}

// Rate limits (plans/ai-salon-assistant.md). The spending ledger is the
// real cap on the bill; these keep one visitor or a flood from using it up.
const (
	// visitorBurst messages at once, then one every visitorEvery.
	visitorBurst = 6
	visitorEvery = 10 * time.Second
	// visitorDaily messages per visitor and day.
	visitorDaily = 40
	// instanceBurst messages at once per instance, then one every
	// instanceEvery: 30 a minute.
	instanceBurst = 30
	instanceEvery = 2 * time.Second
	// maxVisitors bounds the limiter's memory; it starts over beyond it.
	maxVisitors = 10000
)

// visitor is one visitor's rate and today's count.
type visitor struct {
	rate  *rate.Limiter
	day   string
	count int
}

// limiter is the chat's per-visitor and per-instance message limits. It
// lives in this instance's memory: it bounds a flood while the instance
// runs, and the ledger bounds spending across instances and restarts.
type limiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	instance *rate.Limiter
}

func newLimiter() *limiter {
	return &limiter{visitors: map[string]*visitor{}, instance: rate.NewLimiter(rate.Every(instanceEvery), instanceBurst)}
}

// limitReason says why a message was refused: "" when it is allowed.
type limitReason string

const (
	tooFast    limitReason = "rate_limited"
	dailyLimit limitReason = "visitor_daily_limit"
)

// allow counts a message from ip at now.
func (l *limiter) allow(ip string, now time.Time) limitReason {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.visitors) > maxVisitors {
		clear(l.visitors)
	}
	v, ok := l.visitors[ip]
	if !ok {
		v = &visitor{rate: rate.NewLimiter(rate.Every(visitorEvery), visitorBurst)}
		l.visitors[ip] = v
	}
	if day := now.UTC().Format(time.DateOnly); v.day != day {
		v.day, v.count = day, 0
	}
	switch {
	case v.count >= visitorDaily:
		return dailyLimit
	case !v.rate.AllowN(now, 1) || !l.instance.AllowN(now, 1):
		return tooFast
	}
	v.count++
	return ""
}

// clientIP is the visitor's address: the last X-Forwarded-For entry, which
// Cloud Run's front end appends, else the connection's.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// clip is s trimmed to at most n characters.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// cleanHistory keeps the last maxTurns well-formed turns, each clipped.
func cleanHistory(turns []assistant.Turn) []assistant.Turn {
	out := make([]assistant.Turn, 0, len(turns))
	for _, t := range turns {
		if (t.Role == assistant.RoleVisitor || t.Role == assistant.RoleAssistant) && strings.TrimSpace(t.Text) != "" {
			out = append(out, assistant.Turn{Role: t.Role, Text: clip(t.Text, maxTurnText)})
		}
	}
	return out[max(0, len(out)-maxTurns):]
}

// chat answers one visitor message over SSE: it appends the message to the
// log at once, then the assistant's reply with its Book buttons, and
// returns the transcript in chatHistory. Any failure becomes a fixed reply.
// Spending holds and records the chat's model spending (usage.Ledger).
type Spending interface {
	Reserve(ctx context.Context, eur float64) (usage.Reservation, error)
	Settle(ctx context.Context, r usage.Reservation, eur float64) error
}

// settleTimeout bounds recording a reply's cost, which outlives the
// request: a visitor who leaves mid-reply has still been billed.
const settleTimeout = 5 * time.Second

func chat(l i18n.Locale, store *content.Store, a *assistant.Assistant, spend Spending, price usage.Price, limit *limiter, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := views.WithRequest(r.Context(), r.URL)
		r.Body = http.MaxBytesReader(w, r.Body, maxChatBody)
		var sig chatSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		message := clip(sig.ChatInput, maxMessage)
		w.Header().Set("Cache-Control", "no-store")
		sse := datastar.NewSSE(w, r)
		if message == "" {
			return
		}
		history := cleanHistory(sig.ChatHistory)
		n := len(history)
		_ = sse.PatchElementTempl(views.ChatMessage(true, message, nil, nil, nil, n), datastar.WithSelectorID("chat-log"), datastar.WithModeAppend())
		_ = sse.MarshalAndPatchSignals(map[string]any{"chatInput": "", "chatLookup": "", "chatBusy": true})

		start := time.Now()
		var answer assistant.Answer
		var cost float64
		outcome := "answered"
		reason := limit.allow(clientIP(r), start)
		var budget error
		var held usage.Reservation
		// The deadline starts before the reservation, so a slow ledger can't
		// hold the request past it either.
		replyCtx, cancel := context.WithTimeout(ctx, replyTimeout)
		defer cancel()
		if reason == "" {
			// Held before every model call, atomically, at the most a reply can
			// cost: no spending the ledger can't see, even from replies at once.
			held, budget = spend.Reserve(replyCtx, price.Cost(assistant.WorstCaseTokens))
		}
		switch {
		case reason == tooFast:
			answer.Text, outcome = views.ChatTooFast(ctx), string(reason)
		case reason == dailyLimit:
			answer.Text, outcome = views.ChatDailyLimit(ctx), string(reason)
		case errors.Is(budget, usage.ErrMonthCap):
			answer.Text, outcome = views.ChatUnavailable(ctx), "budget_month"
			log.WarnContext(ctx, "chat_budget_reached", "cap", "month")
		case errors.Is(budget, usage.ErrDayCap):
			answer.Text, outcome = views.ChatUnavailable(ctx), "budget_day"
			log.WarnContext(ctx, "chat_budget_reached", "cap", "day")
		case budget != nil:
			answer.Text, outcome = views.ChatUnavailable(ctx), "budget_unreadable"
			log.ErrorContext(ctx, "chat_budget_unreadable", "error", budget.Error())
		default:
			loc := store.Content(replyCtx).In(l)
			k := assistant.Knowledge{Locale: l, Content: loc, Facts: views.AssistantFacts(ctx, loc), Hours: views.AssistantHours(ctx), CareNotes: views.AssistantCareNotes(ctx), Categories: views.AssistantCategories(ctx, loc)}
			for _, qa := range views.AssistantFAQ(ctx, loc) {
				k.FAQ = append(k.FAQ, assistant.QA{Question: qa.Question, Answer: qa.Answer})
			}
			var err error
			if service, search, ok := decodeLookup(sig.ChatLookup); ok {
				answer, err = a.Times(replyCtx, k, service, search)
			} else {
				answer, err = a.Reply(replyCtx, k, history, message)
			}
			cancel()
			// A failed or abandoned reply is billed too: settle whatever it
			// used, on a context the visitor leaving can't cancel. A model call
			// of unknown usage counts at its worst case (assistant.Answer).
			cost = price.Cost(answer.Tokens)
			if answer.Unmetered > 0 {
				log.WarnContext(ctx, "chat_usage_unknown", "model_calls", answer.Unmetered, "eur", cost)
			}
			settleCtx, cancelSettle := context.WithTimeout(context.WithoutCancel(ctx), settleTimeout)
			if recErr := spend.Settle(settleCtx, held, cost); recErr != nil {
				log.ErrorContext(ctx, "chat_spending_not_recorded", "eur", cost, "error", recErr.Error())
			}
			cancelSettle()
			if err != nil {
				outcome = "no_answer"
				if !errors.Is(err, assistant.ErrNoAnswer) {
					outcome = "failed"
					log.ErrorContext(ctx, "chat_failed", "locale", string(l), "error", err.Error())
				}
				answer = assistant.Answer{Text: views.ChatUnavailable(ctx), Tokens: answer.Tokens, Unmetered: answer.Unmetered}
			}
			if answer.Unverified {
				outcome = "unverified"
				answer.Text = strings.TrimSpace(answer.Text + "\n\n" + views.ChatTimesUnverified(ctx))
			}
		}
		// A choice of treatments is asked in the chat's words, never the
		// model's.
		choices := make([]views.ChatChoice, 0, len(answer.Choices))
		for _, c := range answer.Choices {
			choices = append(choices, views.ChatChoice{Service: c.Service, Lookup: encodeLookup(c)})
		}
		if len(choices) > 0 && answer.Text == "" {
			answer.Text = views.ChatChoose(ctx)
		}
		offers := make([]views.ChatOffer, 0, len(answer.Offers))
		for _, o := range answer.Offers {
			offers = append(offers, views.ChatOffer{ServiceID: o.ServiceID, Service: o.Service, StaffID: o.StaffID, Staff: o.Staff})
		}
		// Free times are shown as the lookup found them, never from the
		// model's text.
		times := make([]views.ChatTimes, 0, len(answer.Times))
		for _, ft := range answer.Times {
			ct := views.ChatTimes{Service: ft.Service, Price: ft.Price, Staff: ft.Staff, Unchecked: ft.Unchecked, Failed: ft.Failed}
			for _, s := range ft.Slots {
				ct.Slots = append(ct.Slots, views.ChatSlot{Start: s.Start, Staff: s.Staff})
			}
			times = append(times, ct)
		}
		// Visitors' messages and replies are never logged: only counts.
		log.InfoContext(ctx, "chat_reply", "locale", string(l), "outcome", outcome, "turns", n+1,
			"offers", len(offers), "times", len(times), "choices", len(choices), "duration_ms", time.Since(start).Milliseconds(), "cost_eur", cost,
			"prompt_tokens", answer.Tokens.Prompt, "cached_tokens", answer.Tokens.Cached, "output_tokens", answer.Tokens.Output,
			"unmetered_calls", answer.Unmetered)
		_ = sse.PatchElementTempl(views.ChatMessage(false, answer.Text, times, offers, choices, n+1), datastar.WithSelectorID("chat-log"), datastar.WithModeAppend())
		history = append(history,
			assistant.Turn{Role: assistant.RoleVisitor, Text: message},
			assistant.Turn{Role: assistant.RoleAssistant, Text: views.ChatTranscript(ctx, answer.Text, times, choices)})
		_ = sse.MarshalAndPatchSignals(map[string]any{"chatHistory": history, "chatBusy": false})
	}
}

// encodeLookup is the lookup a choice's button sends back: its treatment
// and the search the visitor asked for, as a query string.
func encodeLookup(c assistant.Choice) string {
	q := url.Values{"service_id": {c.ServiceID}}
	for name, v := range map[string]string{"from_date": c.Search.FromDate, "to_date": c.Search.ToDate, "from_time": c.Search.FromTime, "to_time": c.Search.ToTime} {
		if v != "" {
			q.Set(name, v)
		}
	}
	if c.Search.StaffID != 0 {
		q.Set("staff_id", strconv.FormatInt(c.Search.StaffID, 10))
	}
	return q.Encode()
}

// decodeLookup reads encodeLookup's query; ok is false without a treatment.
// The lookup itself checks the treatment, person, days and times.
func decodeLookup(s string) (service string, search assistant.Search, ok bool) {
	q, err := url.ParseQuery(s)
	if err != nil || strings.TrimSpace(q.Get("service_id")) == "" {
		return "", assistant.Search{}, false
	}
	search = assistant.Search{FromDate: q.Get("from_date"), ToDate: q.Get("to_date"), FromTime: q.Get("from_time"), ToTime: q.Get("to_time")}
	search.StaffID, _ = strconv.ParseInt(q.Get("staff_id"), 10, 64)
	return strings.TrimSpace(q.Get("service_id")), search, true
}
