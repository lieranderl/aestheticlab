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

// limiter allows each visitor a few messages in a burst, then one every
// ten seconds, and all visitors together a bounded rate, so no one can
// run up the model bill.
type limiter struct {
	mu     sync.Mutex
	perIP  map[string]*rate.Limiter
	global *rate.Limiter
}

func newLimiter() *limiter {
	return &limiter{perIP: map[string]*rate.Limiter{}, global: rate.NewLimiter(2, 30)}
}

func (l *limiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.perIP) > 10000 {
		clear(l.perIP)
	}
	v, ok := l.perIP[ip]
	if !ok {
		v = rate.NewLimiter(rate.Every(10*time.Second), 6)
		l.perIP[ip] = v
	}
	return v.Allow() && l.global.Allow()
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
func chat(l i18n.Locale, store *content.Store, a *assistant.Assistant, limit *limiter, log *slog.Logger) http.HandlerFunc {
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
		outcome := "answered"
		if !limit.allow(clientIP(r)) {
			answer.Text, outcome = views.ChatTooFast(ctx), "rate_limited"
		} else {
			loc := store.Content(ctx).In(l)
			k := assistant.Knowledge{Locale: l, Content: loc, Facts: views.AssistantFacts(ctx, loc)}
			for _, qa := range views.AssistantFAQ(ctx, loc) {
				k.FAQ = append(k.FAQ, assistant.QA{Question: qa.Question, Answer: qa.Answer})
			}
			replyCtx, cancel := context.WithTimeout(ctx, replyTimeout)
			var err error
			answer, err = a.Reply(replyCtx, k, history, message)
			cancel()
			if err != nil {
				outcome = "no_answer"
				if !errors.Is(err, assistant.ErrNoAnswer) {
					outcome = "failed"
					log.ErrorContext(ctx, "chat_failed", "locale", string(l), "error", err.Error())
				}
				answer = assistant.Answer{Text: views.ChatUnavailable(ctx)}
			}
		}
		offers := make([]views.ChatOffer, 0, len(answer.Offers))
		for _, o := range answer.Offers {
			offers = append(offers, views.ChatOffer{ServiceID: o.ServiceID, Service: o.Service, StaffID: o.StaffID, Staff: o.Staff})
		}
		// Visitors' messages and replies are never logged: only counts.
		log.InfoContext(ctx, "chat_reply", "locale", string(l), "outcome", outcome, "turns", n+1,
			"offers", len(offers), "duration_ms", time.Since(start).Milliseconds())
		_ = sse.PatchElementTempl(views.ChatMessage(false, answer.Text, offers, n+1), datastar.WithSelectorID("chat-log"), datastar.WithModeAppend())
		history = append(history,
			assistant.Turn{Role: assistant.RoleVisitor, Text: message},
			assistant.Turn{Role: assistant.RoleAssistant, Text: answer.Text})
		_ = sse.MarshalAndPatchSignals(map[string]any{"chatHistory": history, "chatBusy": false})
	}
}
