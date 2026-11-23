package views

import (
	"context"
	"strconv"
	"strings"
	"time"
)

type chatKey struct{}

// WithChat marks ctx's pages as showing the chat assistant.
func WithChat(ctx context.Context) context.Context { return context.WithValue(ctx, chatKey{}, true) }

func chatEnabled(ctx context.Context) bool { on, _ := ctx.Value(chatKey{}).(bool); return on }

// chatSignals are the chat's own signals: the panel is open, the input,
// the visible transcript the server reads back, and a reply is pending.
const chatSignals = `{chatOpen: false, chatInput: '', chatHistory: [], chatBusy: false}`

// chatSuggestions are the common questions the empty chat offers.
func chatSuggestions(ctx context.Context) []string {
	return []string{
		tr(ctx, "What do your treatments cost?"),
		tr(ctx, "When can I book a manicure?"),
		tr(ctx, "Tell me about the studio"),
		tr(ctx, "How does laser hair removal work?"),
	}
}

// ChatChoice is a treatment to choose under an assistant reply, when the
// visitor asked for a kind with several options.
type ChatChoice struct {
	ServiceID string
	Service   string
}

// ChatChoose is the chat's own question over a choice of treatments.
func ChatChoose(ctx context.Context) string {
	return tr(ctx, "Which one would you like? Pick a treatment to see its free times.")
}

// chatChoiceAsk is what a choice's button sends: a request for its times.
func chatChoiceAsk(ctx context.Context, c ChatChoice) string {
	return trf(ctx, "When can I book %v?", c.Service)
}

// ChatOffer is a Book button under an assistant reply: a treatment, with
// one member of staff or anyone.
type ChatOffer struct {
	ServiceID string
	Service   string
	StaffID   int64
	Staff     string
}

// ChatTimes are the free times under an assistant reply, as the lookup
// found them: a treatment's, with one member of staff or anyone.
type ChatTimes struct {
	Service string
	Price   *float64
	// Staff is the person the lookup was for; empty for anyone.
	Staff  string
	Slots  []ChatSlot
	Failed bool
	// Unchecked are the people whose times could not be checked.
	Unchecked []string
}

// ChatSlot is a free start time, in the studio's time zone, and the person
// free then.
type ChatSlot struct {
	Start time.Time
	Staff string
}

// chatTimesHeading names the treatment (and person) the times are for.
func chatTimesHeading(ctx context.Context, t ChatTimes) string {
	if t.Staff != "" {
		return trf(ctx, "Free times for %[1]v with %[2]v", t.Service, t.Staff)
	}
	return trf(ctx, "Free times for %v", t.Service)
}

func chatTimesPrice(ctx context.Context, t ChatTimes) string {
	if t.Price == nil {
		return ""
	}
	return price(ctx, *t.Price)
}

// chatSlotWhen is a free time as the chat lists it: "Mon 23/11 · 10:00".
func chatSlotWhen(ctx context.Context, s ChatSlot) string {
	days := [...]string{
		tr(ctx, "Sun"), tr(ctx, "Mon"), tr(ctx, "Tue"), tr(ctx, "Wed"), tr(ctx, "Thu"), tr(ctx, "Fri"), tr(ctx, "Sat"),
	}
	return days[s.Start.Weekday()] + " " + s.Start.Format("02/01") + " · " + s.Start.Format("15:04")
}

// chatSlotStaff is who is free then, unless the lookup was for one person.
func chatSlotStaff(t ChatTimes, s ChatSlot) string {
	if t.Staff != "" || s.Staff == "" {
		return ""
	}
	return " · " + s.Staff
}

// chatTimesStatus describes a search without any verified slots.
func chatTimesStatus(ctx context.Context, t ChatTimes) string {
	if t.Failed {
		return tr(ctx, "We could not check availability. Please check the booking page.")
	}
	if len(t.Slots) == 0 {
		return tr(ctx, "No free times found for that search.")
	}
	return ""
}

func chatHasSlots(times []ChatTimes) bool {
	for _, t := range times {
		if len(t.Slots) > 0 {
			return true
		}
	}
	return false
}

// chatUnchecked says whose times could not be checked; empty if none.
func chatUnchecked(ctx context.Context, t ChatTimes) string {
	if len(t.Unchecked) == 0 {
		return ""
	}
	return trf(ctx, "%v could not be checked: the booking page shows their times.", strings.Join(t.Unchecked, ", "))
}

func chatTimesNote(ctx context.Context) string {
	return tr(ctx, "These times look free now; the booking page confirms them.")
}

// ChatTranscript is an assistant reply as the transcript keeps it: its
// text, then its free times as plain lines, so a restored chat and the
// model's next turn see the times the visitor saw.
func ChatTranscript(ctx context.Context, text string, times []ChatTimes, choices []ChatChoice) string {
	parts := []string{}
	if text != "" {
		parts = append(parts, text)
	}
	if len(choices) > 0 {
		lines := make([]string, 0, len(choices))
		for _, c := range choices {
			lines = append(lines, "• "+c.Service)
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	for _, t := range times {
		lines := []string{chatTimesHeading(ctx, t) + ":"}
		if amount := chatTimesPrice(ctx, t); amount != "" {
			lines = append(lines, amount)
		}
		for _, s := range t.Slots {
			lines = append(lines, "• "+chatSlotWhen(ctx, s)+chatSlotStaff(t, s))
		}
		if status := chatTimesStatus(ctx, t); status != "" {
			lines = append(lines, status)
		}
		if u := chatUnchecked(ctx, t); u != "" {
			lines = append(lines, u)
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	if chatHasSlots(times) {
		parts = append(parts, chatTimesNote(ctx))
	}
	return strings.Join(parts, "\n\n")
}

// chatBooking is o as a Book button; n keeps its id unique on the page.
func chatBooking(ctx context.Context, o ChatOffer, n int) Booking {
	b := Booking{
		ID:          "chat-book-" + strconv.Itoa(n) + "-" + o.ServiceID,
		Product:     o.ServiceID,
		Text:        trf(ctx, "Book %v", o.Service),
		Placement:   "chat",
		ServiceID:   o.ServiceID,
		ServiceName: o.Service,
		Class:       "btn btn-accent btn-sm h-auto min-h-10 py-2 text-left font-main text-xs font-semibold normal-case leading-snug",
	}
	if o.StaffID != 0 {
		b.ID += "-" + strconv.FormatInt(o.StaffID, 10)
		b.Staff = strconv.FormatInt(o.StaffID, 10)
		b.Text = trf(ctx, "Book %[1]v with %[2]v", o.Service, o.Staff)
	}
	return b
}

// ChatUnavailable is the reply when the assistant cannot answer.
func ChatUnavailable(ctx context.Context) string {
	return tr(ctx, "Sorry, I can't answer right now. Please message us on Instagram or by email.")
}

// ChatTimesUnverified is added to a reply whose text said when an
// appointment is free without checking the calendar, in its place.
func ChatTimesUnverified(ctx context.Context) string {
	return tr(ctx, "I can only confirm free times by checking the calendar. Tell me which treatment you'd like and I'll look them up.")
}

// ChatTooFast is the reply when a visitor sends messages too quickly.
func ChatTooFast(ctx context.Context) string {
	return tr(ctx, "You're sending messages too quickly. Please wait a minute and try again.")
}

// ChatDailyLimit is the reply when a visitor has sent today's messages.
func ChatDailyLimit(ctx context.Context) string {
	return tr(ctx, "You've reached today's message limit for the assistant. Please message us on Instagram or by email.")
}
