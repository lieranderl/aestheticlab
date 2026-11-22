package views

import (
	"context"
	"strconv"
)

type chatKey struct{}

// WithChat marks ctx's pages as showing the chat assistant.
func WithChat(ctx context.Context) context.Context { return context.WithValue(ctx, chatKey{}, true) }

func chatEnabled(ctx context.Context) bool { on, _ := ctx.Value(chatKey{}).(bool); return on }

// chatSignals are the chat's own signals: the panel is open, the input,
// the visible transcript the server reads back, and a reply is pending.
const chatSignals = `{chatOpen: false, chatInput: '', chatHistory: [], chatBusy: false}`

// ChatOffer is a Book button under an assistant reply: a treatment, with
// one member of staff or anyone.
type ChatOffer struct {
	ServiceID string
	Service   string
	StaffID   int64
	Staff     string
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

// ChatTooFast is the reply when a visitor sends messages too quickly.
func ChatTooFast(ctx context.Context) string {
	return tr(ctx, "You're sending messages too quickly. Please wait a minute and try again.")
}
