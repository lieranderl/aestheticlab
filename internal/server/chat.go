package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
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
	// replyTimeout bounds one answer, tool calls included.
	replyTimeout = 30 * time.Second
)

// chatSignals are the signals a chat request carries (views.chatSignals).
type chatSignals struct {
	ChatInput   string           `json:"chatInput"`
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
		_ = sse.PatchElementTempl(views.ChatMessage(true, message, nil, n), datastar.WithSelectorID("chat-log"), datastar.WithModeAppend())
		_ = sse.MarshalAndPatchSignals(map[string]any{"chatInput": "", "chatBusy": true})

		start := time.Now()
		var answer assistant.Answer
		var cost float64
		outcome := "answered"
		reason := limit.allow(clientIP(r), start)
		var budget error
		var held usage.Reservation
		if reason == "" {
			// Held before every model call, atomically, at the most a reply can
			// cost: no spending the ledger can't see, even from replies at once.
			held, budget = spend.Reserve(ctx, price.Cost(assistant.WorstCaseTokens))
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
			loc := store.Content(ctx).In(l)
			k := assistant.Knowledge{Locale: l, Content: loc, Facts: views.AssistantFacts(ctx, loc), CareNotes: views.AssistantCareNotes(ctx), Categories: views.AssistantCategories(ctx, loc)}
			for _, qa := range views.AssistantFAQ(ctx, loc) {
				k.FAQ = append(k.FAQ, assistant.QA{Question: qa.Question, Answer: qa.Answer})
			}
			replyCtx, cancel := context.WithTimeout(ctx, replyTimeout)
			var err error
			answer, err = a.Reply(replyCtx, k, history, message)
			cancel()
			// A failed or abandoned reply is billed too: settle whatever it
			// used, on a context the visitor leaving can't cancel.
			cost = price.Cost(answer.Tokens)
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
				answer = assistant.Answer{Text: views.ChatUnavailable(ctx), Tokens: answer.Tokens}
			}
		}
		offers := make([]views.ChatOffer, 0, len(answer.Offers))
		for _, o := range answer.Offers {
			offers = append(offers, views.ChatOffer{ServiceID: o.ServiceID, Service: o.Service, StaffID: o.StaffID, Staff: o.Staff})
		}
		// Visitors' messages and replies are never logged: only counts.
		log.InfoContext(ctx, "chat_reply", "locale", string(l), "outcome", outcome, "turns", n+1,
			"offers", len(offers), "duration_ms", time.Since(start).Milliseconds(), "cost_eur", cost,
			"prompt_tokens", answer.Tokens.Prompt, "cached_tokens", answer.Tokens.Cached, "output_tokens", answer.Tokens.Output)
		_ = sse.PatchElementTempl(views.ChatMessage(false, answer.Text, offers, n+1), datastar.WithSelectorID("chat-log"), datastar.WithModeAppend())
		history = append(history,
			assistant.Turn{Role: assistant.RoleVisitor, Text: message},
			assistant.Turn{Role: assistant.RoleAssistant, Text: answer.Text})
		_ = sse.MarshalAndPatchSignals(map[string]any{"chatHistory": history, "chatBusy": false})
	}
}
