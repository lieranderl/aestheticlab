package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/genai"

	"github.com/lieranderl/aestheticlab/internal/assistant"
	"github.com/lieranderl/aestheticlab/internal/content"
	"github.com/lieranderl/aestheticlab/internal/server"
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

func chatSite(t *testing.T, m assistant.Model) http.Handler {
	t.Helper()
	return server.New(server.Config{Content: bookableContent(t), Assistant: assistant.New(assistant.Options{Model: m})})
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
