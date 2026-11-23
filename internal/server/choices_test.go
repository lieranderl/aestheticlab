package server_test

import (
	"context"
	"html"
	"os"
	"strings"
	"testing"

	"google.golang.org/genai"

	"github.com/lieranderl/aestheticlab/internal/assistant"
	"github.com/lieranderl/aestheticlab/internal/content"
	"github.com/lieranderl/aestheticlab/internal/server"
)

// choosing is a model that offers a choice of the two manicures, writing
// prose the chat must drop.
type choosing struct{ args map[string]any }

func (c choosing) Generate(context.Context, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content: genai.NewContentFromParts([]*genai.Part{
			{Text: "Sure, tomorrow works! Which one?"},
			{FunctionCall: &genai.FunctionCall{ID: "1", Name: "choose_treatment", Args: c.args}},
		}, genai.RoleModel),
		FinishReason: genai.FinishReasonStop,
	}}}, nil
}

// manicureContent is the snapshot with Manicure and Manicure+polish
// bookable with Zara.
func manicureContent(t *testing.T) *content.Store {
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
		if s["id"] == float64(4847894) || s["id"] == float64(4887841) {
			s["staff_ids"] = []any{float64(642297)}
		}
	}
	return content.NewStore(content.Options{Source: docsSource{docs}})
}

// Asked for a kind with several options, the chat asks which one in its
// own words, never the model's, with a button for each treatment; a button
// asks for that treatment's free times. The transcript keeps the options.
func TestAChoiceOfTreatmentsIsAskedWithButtons(t *testing.T) {
	both := []any{"4847894", "4887841"}
	for name, tc := range map[string]struct {
		path string
		args map[string]any
		want []string
	}{
		"in English": {
			path: "/en-BE/chat",
			args: map[string]any{"service_ids": both},
			want: []string{"Which one would you like? Pick a treatment to see its free times.", `aria-label="Treatments to choose from"`, ">Manicure<", ">Manicure+polish<", "When can I book Manicure?", "When can I book Manicure+polish?", "$chatService = '4887841'", `"chatHistory"`, "• Manicure+polish"},
		},
		"in Dutch": {
			path: "/nl-BE/chat",
			args: map[string]any{"service_ids": both},
			want: []string{"Welke wil je? Kies een behandeling om de vrije tijden te zien.", ">Manicure + lak<", "Wanneer kan ik Manicure + lak boeken?", "$chatService = '4887841'"},
		},
	} {
		h := server.New(server.Config{Content: manicureContent(t), Assistant: assistant.New(assistant.Options{Model: choosing{tc.args}}), Spending: &spending{}, ChatPrice: testPrice})
		body := html.UnescapeString(post(t, h, tc.path, `{"chatInput":"When can I book a manicure?"}`, nil).Body.String())
		for _, w := range tc.want {
			if !strings.Contains(body, w) {
				t.Errorf("%s: the reply lacks %q", name, w)
			}
		}
		if strings.Contains(body, "tomorrow works") {
			t.Errorf("%s: the model's own text is shown", name)
		}
	}
}

// A choice's button sends its treatment's ID with the message: the chat
// shows that treatment's times without asking the model which it meant.
func TestAChoicesButtonShowsItsOwnTreatmentsTimes(t *testing.T) {
	m := &counting{}
	h := server.New(server.Config{Content: manicureContent(t), Assistant: assistant.New(assistant.Options{Model: m}), Spending: &spending{}, ChatPrice: testPrice})
	body := html.UnescapeString(post(t, h, "/nl-BE/chat", `{"chatInput":"Wanneer kan ik Manicure + lak boeken?","chatService":"4887841"}`, nil).Body.String())
	for _, w := range []string{"Wanneer kan ik Manicure + lak boeken?", "Vrije tijden voor Manicure + lak", `"chatService":""`} {
		if !strings.Contains(body, w) {
			t.Errorf("the reply lacks %q", w)
		}
	}
	if m.calls != 0 {
		t.Errorf("%d model calls, want none", m.calls)
	}
}
