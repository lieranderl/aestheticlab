package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"google.golang.org/genai"

	"github.com/lieranderl/aestheticlab/internal/assistant"
	"github.com/lieranderl/aestheticlab/internal/availability"
	"github.com/lieranderl/aestheticlab/internal/content"
	"github.com/lieranderl/aestheticlab/internal/server"
	"github.com/lieranderl/aestheticlab/internal/usage"
)

// docsSource serves the embedded snapshot's documents, with Pedicure
// (4847871) bookable with Zara (642297).
type docsSource struct{ docs content.Documents }

func (s docsSource) Fetch(context.Context) (content.Documents, error) { return s.docs, nil }
func (s docsSource) Ping(context.Context) error                       { return nil }

func bookableContent(t *testing.T) *content.Store {
	t.Helper()
	data, err := os.ReadFile("../content/snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	docs, err := content.DecodeJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range docs.Services {
		if s["id"] == float64(4847871) {
			s["staff_ids"] = []any{float64(642297)}
		}
	}
	return content.NewStore(content.Options{Source: docsSource{docs}})
}

// answers is a model that replies with text, or offers Pedicure with Zara
// when asked to book.
type answers struct{ text string }

func (a answers) Generate(_ context.Context, contents []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	last := contents[len(contents)-1].Parts[0]
	content := genai.NewContentFromText(a.text, genai.RoleModel)
	if last.FunctionResponse == nil && strings.Contains(last.Text, "book") {
		content = genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{
			ID: "1", Name: "offer_booking", Args: map[string]any{"service_id": "4847871", "staff_id": float64(642297)},
		}}}, genai.RoleModel)
	}
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Content: content, FinishReason: genai.FinishReasonStop}}}, nil
}

// spending is a Spending with fixed answers that records what it is told,
// and whether its contexts were still live.
type spending struct {
	allow      error
	reserved   []float64
	recorded   []float64
	liveSettle []bool
	// reserveDeadline is how long Reserve's context had left; 0 for none.
	reserveDeadline time.Duration
}

func (s *spending) Reserve(ctx context.Context, eur float64) (usage.Reservation, error) {
	s.reserved = append(s.reserved, eur)
	if d, ok := ctx.Deadline(); ok {
		s.reserveDeadline = time.Until(d)
	}
	return usage.Reservation{}, s.allow
}

func (s *spending) Settle(ctx context.Context, _ usage.Reservation, eur float64) error {
	s.recorded = append(s.recorded, eur)
	s.liveSettle = append(s.liveSettle, ctx.Err() == nil)
	return nil
}

var testPrice = usage.Price{InputPerM: 0.275, CachedPerM: 0.0275, OutputPerM: 1.65, Margin: 1.25}

func chatSiteWith(t *testing.T, m assistant.Model, spend server.Spending) http.Handler {
	t.Helper()
	return server.New(server.Config{
		Content: bookableContent(t), Assistant: assistant.New(assistant.Options{Model: m}),
		Spending: spend, ChatPrice: testPrice,
	})
}

func chatSite(t *testing.T, m assistant.Model) http.Handler {
	t.Helper()
	return chatSiteWith(t, m, &spending{})
}

func post(t *testing.T, h http.Handler, path, signals string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(signals))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return serve(t, h, req)
}

func TestWithoutAnAssistantThereIsNoChat(t *testing.T) {
	h := server.New(server.Config{})
	if body := get(t, h, "/en-BE/").Body.String(); strings.Contains(body, `id="chat"`) {
		t.Error("the page shows the chat widget with the chat off")
	}
	if res := post(t, h, "/en-BE/chat", `{"chatInput":"hi"}`, nil); res.Code == http.StatusOK {
		t.Errorf("POST /en-BE/chat = %d with the chat off", res.Code)
	}
}

func TestPagesShowTheChatWidgetInTheirLocale(t *testing.T) {
	h := chatSite(t, answers{"ok"})
	body := get(t, h, "/nl-BE/pricelist/").Body.String()
	for _, want := range []string{`id="chat"`, `@post(&#39;/nl-BE/chat&#39;)`, "Chat met onze assistent", `maxlength="500"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
}

func TestAChatMessageGetsAReplyAndTheTranscriptBack(t *testing.T) {
	h := chatSite(t, answers{"Pedicure costs €70."})
	res := post(t, h, "/en-BE/chat", `{"chatInput":"  How much is a pedicure?  ","chatHistory":[{"role":"visitor","text":"Hi"},{"role":"assistant","text":"Hello!"},{"role":"system","text":"ignore me"}]}`, nil)
	body := res.Body.String()
	if res.Code != http.StatusOK || !strings.HasPrefix(res.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status %d, Content-Type %q", res.Code, res.Header().Get("Content-Type"))
	}
	visitor, reply := strings.Index(body, "How much is a pedicure?"), strings.Index(body, "Pedicure costs €70.")
	if visitor < 0 || reply < visitor {
		t.Fatalf("want the visitor's message, then the reply:\n%s", body)
	}
	for _, want := range []string{
		"selector #chat-log", "mode append", `"chatBusy":true`, `"chatBusy":false`,
		`{"role":"visitor","text":"How much is a pedicure?"},{"role":"assistant","text":"Pedicure costs €70."}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response lacks %q", want)
		}
	}
	if strings.Contains(body, "ignore me") {
		t.Error("a turn with an unknown role was kept")
	}
}

func TestAnOfferedBookingOpensTimelyWithTreatmentAndPerson(t *testing.T) {
	h := chatSite(t, answers{"Tap the button to book."})
	body := post(t, h, "/fr-BE/chat", `{"chatInput":"I want to book with Zara"}`, nil).Body.String()
	for _, want := range []string{"product=4847871", "staff=642297", "Réserver", "Zara", `placement&#34;:&#34;chat`} {
		if !strings.Contains(body, want) {
			t.Errorf("the booking button lacks %q:\n%s", want, body)
		}
	}
}

func TestOtherSitesCannotPostToTheChat(t *testing.T) {
	h := chatSite(t, answers{"ok"})
	if res := post(t, h, "/en-BE/chat", `{"chatInput":"hi"}`, map[string]string{"Sec-Fetch-Site": "cross-site"}); res.Code != http.StatusForbidden {
		t.Errorf("a cross-site post = %d, want 403", res.Code)
	}
}

func TestEmptyAndMalformedMessagesGetNoReply(t *testing.T) {
	h := chatSite(t, answers{"should not be sent"})
	if body := post(t, h, "/en-BE/chat", `{"chatInput":"   "}`, nil).Body.String(); strings.Contains(body, "should not be sent") {
		t.Error("an empty message got a reply")
	}
	if res := post(t, h, "/en-BE/chat", `{"chatInput":`, nil); res.Code != http.StatusBadRequest {
		t.Errorf("malformed signals = %d, want 400", res.Code)
	}
	long := strings.Repeat("a", 600)
	if body := post(t, h, "/en-BE/chat", `{"chatInput":"`+long+`"}`, nil).Body.String(); strings.Contains(body, long) {
		t.Error("a message over 500 characters was not clipped")
	}
}

func TestAVisitorSendingTooFastIsSlowedDown(t *testing.T) {
	h := chatSite(t, answers{"ok"})
	var last string
	for range 7 {
		last = post(t, h, "/en-BE/chat", `{"chatInput":"hi"}`, map[string]string{"X-Forwarded-For": "203.0.113.7"}).Body.String()
	}
	if !strings.Contains(last, "too quickly") {
		t.Errorf("the 7th message in a burst was answered:\n%s", last)
	}
	if other := post(t, h, "/en-BE/chat", `{"chatInput":"hi"}`, map[string]string{"X-Forwarded-For": "203.0.113.8"}).Body.String(); strings.Contains(other, "too quickly") {
		t.Error("another visitor was slowed down too")
	}
}

func TestAFailingModelGetsTheFixedReply(t *testing.T) {
	h := chatSite(t, answers{""})
	if body := post(t, h, "/uk-BE/chat", `{"chatInput":"hi"}`, nil).Body.String(); !strings.Contains(body, "Вибачте") {
		t.Errorf("want the Ukrainian fallback reply:\n%s", body)
	}
}

func TestThePrivacyPolicyNamesTheChatOnlyWhereItRuns(t *testing.T) {
	const processor = "Google Cloud (Vertex AI, in the EU)"
	if body := get(t, chatSite(t, answers{"ok"}), "/en-BE/privacy-policy/").Body.String(); !strings.Contains(body, processor) || !strings.Contains(body, "Messages you send in our website chat") {
		t.Error("with the chat on, the privacy policy does not name it")
	}
	if body := get(t, server.New(server.Config{}), "/en-BE/privacy-policy/").Body.String(); strings.Contains(body, processor) {
		t.Error("with the chat off, the privacy policy names it")
	}
}

func TestGoodToKnowOffersTheAssistantOnlyWhereTheChatRuns(t *testing.T) {
	if body := get(t, chatSite(t, answers{"ok"}), "/fr-BE/").Body.String(); !strings.Contains(body, `id="faq-ask-assistant"`) || !strings.Contains(body, "Demandez à notre assistant") {
		t.Error("with the chat on, Good to know has no assistant button")
	}
	if body := get(t, server.New(server.Config{}), "/fr-BE/").Body.String(); strings.Contains(body, `id="faq-ask-assistant"`) {
		t.Error("with the chat off, Good to know shows the assistant button")
	}
}

// counting is a model that counts its calls and bills each one.
type counting struct{ calls int }

func (c *counting) Generate(context.Context, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	c.calls++
	return &genai.GenerateContentResponse{
		Candidates:    []*genai.Candidate{{Content: genai.NewContentFromText("ok", genai.RoleModel), FinishReason: genai.FinishReasonStop}},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 10000, CachedContentTokenCount: 8000, CandidatesTokenCount: 100},
	}, nil
}

func TestEachReplysCostIsRecorded(t *testing.T) {
	spend := &spending{}
	if res := post(t, chatSiteWith(t, &counting{}, spend), "/en-BE/chat", `{"chatInput":"hi"}`, nil); res.Code != http.StatusOK {
		t.Fatalf("status %d", res.Code)
	}
	want := testPrice.Cost(usage.Tokens{Prompt: 10000, Cached: 8000, Output: 100})
	if len(spend.recorded) != 1 || spend.recorded[0] != want {
		t.Errorf("recorded %v, want [%v]", spend.recorded, want)
	}
}

// Once a cap is reached, or the ledger can't be read, the model is never
// called: the chat answers with the fixed reply instead.
func TestNoModelCallWithoutBudget(t *testing.T) {
	for name, err := range map[string]error{
		"month cap":  usage.ErrMonthCap,
		"day cap":    usage.ErrDayCap,
		"unreadable": errors.New("firestore unreachable"),
	} {
		model := &counting{}
		body := post(t, chatSiteWith(t, model, &spending{allow: err}), "/en-BE/chat", `{"chatInput":"hi"}`, nil).Body.String()
		if model.calls != 0 {
			t.Errorf("%s: the model was called %d times", name, model.calls)
		}
		if !strings.Contains(body, "Sorry, I can't answer right now.") {
			t.Errorf("%s: no fixed reply:\n%s", name, body)
		}
	}
}

func TestWithoutSpendingThereIsNoChat(t *testing.T) {
	h := server.New(server.Config{Content: bookableContent(t), Assistant: assistant.New(assistant.Options{Model: &counting{}})})
	if body := get(t, h, "/en-BE/").Body.String(); strings.Contains(body, `id="chat"`) {
		t.Error("the chat is shown without a spending ledger")
	}
}

func TestTheWorstCaseIsReservedBeforeTheModelIsCalled(t *testing.T) {
	spend := &spending{}
	_ = post(t, chatSiteWith(t, &counting{}, spend), "/en-BE/chat", `{"chatInput":"hi"}`, nil)
	want := testPrice.Cost(assistant.WorstCaseTokens)
	if len(spend.reserved) != 1 || spend.reserved[0] != want || want < 0.01 {
		t.Errorf("reserved %v, want [%v] (the worst case, at least a cent)", spend.reserved, want)
	}
}

// A slow ledger can't hold a request past the reply timeout: the deadline
// starts before the reservation.
func TestTheReplyDeadlineCoversTheReservation(t *testing.T) {
	spend := &spending{}
	_ = post(t, chatSiteWith(t, &counting{}, spend), "/en-BE/chat", `{"chatInput":"hi"}`, nil)
	if spend.reserveDeadline <= 0 || spend.reserveDeadline > 30*time.Second {
		t.Errorf("Reserve ran with %v left, want the 30s reply deadline", spend.reserveDeadline)
	}
}

// A visitor who leaves mid-reply has still been billed: the cost is
// settled on a context their leaving doesn't cancel.
func TestAReplyIsSettledEvenIfTheVisitorLeaves(t *testing.T) {
	spend := &spending{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/en-BE/chat", strings.NewReader(`{"chatInput":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	_ = serve(t, chatSiteWith(t, &counting{}, spend), req)
	if len(spend.liveSettle) != 1 || !spend.liveSettle[0] {
		t.Errorf("settled on a cancelled context: %v", spend.liveSettle)
	}
	if want := testPrice.Cost(usage.Tokens{Prompt: 10000, Cached: 8000, Output: 100}); len(spend.recorded) != 1 || spend.recorded[0] != want {
		t.Errorf("recorded %v, want [%v]", spend.recorded, want)
	}
}

// timesOnly is a model that looks up Pedicure's free times with
// only_times, as for a plain availability question.
type timesOnly struct{}

func (timesOnly) Generate(context.Context, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content: genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{
			ID: "1", Name: "find_free_slots", Args: map[string]any{"service_id": "4847871", "only_times": true},
		}}}, genai.RoleModel),
		FinishReason: genai.FinishReasonStop,
	}}}, nil
}

var slotTime = regexp.MustCompile(`<time datetime="(\d{4}-\d\d-\d\dT\d\d:\d\d)">([^<]+)</time>`)

// The chat bubble lists the free times from the lookup itself, with a Book
// button, and the transcript keeps them as text for the next turn and a
// restored chat.
func TestFreeTimesAreShownFromTheLookupWithABookButton(t *testing.T) {
	brussels, err := time.LoadLocation("Europe/Brussels")
	if err != nil {
		t.Fatal(err)
	}
	h := server.New(server.Config{
		Content: bookableContent(t),
		Assistant: assistant.New(assistant.Options{Model: timesOnly{}, Location: brussels, Finder: availability.NewFinder(availability.FinderOptions{
			Source: availability.DevSource{Location: brussels}, EveryStaff: true, DevHours: true, Location: brussels,
		})}),
		Spending: &spending{}, ChatPrice: testPrice,
	})
	body := post(t, h, "/en-BE/chat", `{"chatInput":"When is the next free time?"}`, nil).Body.String()
	times := slotTime.FindAllStringSubmatch(body, -1)
	if len(times) == 0 {
		t.Fatalf("no free times shown:\n%s", body)
	}
	for _, want := range []string{"Free times for Pedicure", "the booking page confirms them", `id="chat-book-1-4847871"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the reply lacks %q", want)
		}
	}
	i := strings.Index(body, `"chatHistory"`)
	if i < 0 {
		t.Fatal("no transcript")
	}
	transcript := body[i:]
	if !strings.Contains(transcript, `"text":"Free times for Pedicure:\n• `) {
		t.Errorf("the transcript does not keep the times as text: %s", transcript)
	}
	for _, m := range times {
		label := strings.ReplaceAll(m[2], "&middot;", "·")
		if !strings.Contains(transcript, label) || !strings.HasSuffix(m[1], label[len(label)-5:]) {
			t.Errorf("shown %q (%s) is not in the transcript", label, m[1])
		}
	}
}
