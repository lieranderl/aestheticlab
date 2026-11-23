package assistant_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"google.golang.org/genai"

	"github.com/lieranderl/aestheticlab/internal/assistant"
	"github.com/lieranderl/aestheticlab/internal/content"
)

// manicures are the studio's manicures, bookable with Zara, as service IDs
// 9000 onwards; "Extensions using dual forms" is not bookable online.
var manicures = []string{"Manicure", "Manicure+polish", "Manicure + gel polish color (short nails)", "Manicure + gel polish nude (long nails)", "Express manicure + polish", "Male manicure"}

// withManicures is k with the manicures added.
func withManicures(k assistant.Knowledge) assistant.Knowledge {
	c := *k.Content
	c.ServiceGroups = append(slices.Clone(c.ServiceGroups), content.ServiceGroup{ID: "2", Name: "Manicure", NameEN: "Manicure"})
	c.Services = slices.Clone(c.Services)
	for i, n := range manicures {
		c.Services = append(c.Services, content.Service{ID: fmt.Sprint(9000 + i), GroupID: "2", Name: n, NameEN: n, Duration: 60, Price: 40, StaffIDs: []int64{642297}})
	}
	c.Services = append(c.Services, content.Service{ID: "9100", GroupID: "2", Name: "Extensions using dual forms", NameEN: "Extensions using dual forms", Duration: 120, Price: 90})
	k.Content = &c
	return k
}

// callSaying is a response that calls name and writes s with it.
func callSaying(name string, args map[string]any, s string) *genai.GenerateContentResponse {
	resp := call(name, args)
	resp.Candidates[0].Content.Parts = append([]*genai.Part{{Text: s}}, resp.Candidates[0].Content.Parts...)
	return resp
}

// Asked when they can book a kind of treatment with several options, the
// model calls choose_treatment: the chat asks which itself, with a button
// for each, and drops whatever the model wrote, so no wording of its can
// say a time is free. The choice ends the reply.
func TestAKindWithSeveralOptionsBecomesAChoice(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{
		callSaying("choose_treatment", map[string]any{"service_ids": []any{"9000", "9001", "9002"}}, "Sure, Zara is free tomorrow! Which manicure?"),
		text("never asked for"),
	}}
	got, err := newAssistant(m).Reply(context.Background(), withManicures(knowledge()), nil, "When can I book a manicure?")
	if err != nil {
		t.Fatal(err)
	}
	want := []assistant.Choice{{ServiceID: "9000", Service: "Manicure"}, {ServiceID: "9001", Service: "Manicure+polish"}, {ServiceID: "9002", Service: "Manicure + gel polish color (short nails)"}}
	if !slices.Equal(got.Choices, want) {
		t.Errorf("choices = %+v, want %+v", got.Choices, want)
	}
	if got.Text != "" || got.Unverified || len(got.Times) > 0 {
		t.Errorf("text %q, unverified %v, %d times; want only the choice", got.Text, got.Unverified, len(got.Times))
	}
	if len(m.requests) != 1 {
		t.Errorf("%d model calls, want 1: the choice ends the reply", len(m.requests))
	}
}

// Only treatments bookable online are offered, each once, at most eight.
// Fewer than two is no choice: the model is told so and answers itself.
func TestAChoiceOffersOnlyBookableTreatments(t *testing.T) {
	k := withManicures(knowledge())
	for name, tc := range map[string]struct {
		args map[string]any
		want []assistant.Choice
	}{
		"unknown, unbookable and repeated": {
			args: map[string]any{"service_ids": []any{"nope", "9100", "9000", "9000", "9005"}},
			want: []assistant.Choice{{ServiceID: "9000", Service: "Manicure"}, {ServiceID: "9005", Service: "Male manicure"}},
		},
		"too few": {
			args: map[string]any{"service_ids": []any{"9000", "9100"}},
		},
	} {
		m := &scripted{replies: []*genai.GenerateContentResponse{call("choose_treatment", tc.args), text("Which manicure would you like?")}}
		got, err := newAssistant(m).Reply(context.Background(), k, nil, "When can I book a manicure?")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !slices.Equal(got.Choices, tc.want) {
			t.Errorf("%s: choices = %+v, want %+v", name, got.Choices, tc.want)
		}
		if tc.want == nil && got.Text != "Which manicure would you like?" {
			t.Errorf("%s: text %q, want the model's own question", name, got.Text)
		}
	}

	ids := make([]any, 0, 12)
	for i := range 12 {
		ids = append(ids, fmt.Sprint(9000+i%len(manicures)))
	}
	big := withManicures(knowledge())
	c := *big.Content
	for i := range 6 {
		c.Services = append(slices.Clone(c.Services), content.Service{ID: fmt.Sprint(9200 + i), GroupID: "2", Name: fmt.Sprint("Nail art ", i), NameEN: fmt.Sprint("Nail art ", i), StaffIDs: []int64{642297}})
		ids = append(ids, fmt.Sprint(9200+i))
	}
	big.Content = &c
	m := &scripted{replies: []*genai.GenerateContentResponse{call("choose_treatment", map[string]any{"service_ids": ids})}}
	got, _ := newAssistant(m).Reply(context.Background(), big, nil, "When can I book a manicure?")
	if len(got.Choices) != 8 {
		t.Errorf("%d choices, want at most 8", len(got.Choices))
	}
}

// The prompt tells the model to offer a choice rather than ask in words.
func TestThePromptOffersAChoiceOfTreatments(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{text("ok")}}
	_, _ = newAssistant(m).Reply(context.Background(), knowledge(), nil, "?")
	cfg := m.configs[0]
	if !slices.ContainsFunc(cfg.Tools[0].FunctionDeclarations, func(f *genai.FunctionDeclaration) bool { return f.Name == "choose_treatment" }) {
		t.Error("the model has no choose_treatment tool")
	}
	if prompt := cfg.SystemInstruction.Parts[0].Text; !strings.Contains(prompt, "call choose_treatment") {
		t.Error("the prompt doesn't say when to call choose_treatment")
	}
}

// A choice's button looks up its own treatment, without the model: the
// times shown are always that treatment's, whatever its name in the
// visitor's language.
func TestAChoicesButtonLooksUpItsOwnTreatment(t *testing.T) {
	m := &scripted{}
	a := newAssistant(m)
	got, err := a.Times(context.Background(), withManicures(knowledge()), "9001")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Times) != 1 || got.Times[0].ServiceID != "9001" || got.Times[0].Service != "Manicure+polish" {
		t.Errorf("times = %+v, want Manicure+polish's", got.Times)
	}
	if len(got.Offers) != 1 || got.Offers[0].ServiceID != "9001" {
		t.Errorf("offers = %+v, want a Book button for Manicure+polish", got.Offers)
	}
	if len(m.requests) != 0 {
		t.Errorf("%d model calls, want none", len(m.requests))
	}
	if _, err := a.Times(context.Background(), withManicures(knowledge()), "nope"); err == nil {
		t.Error("an unknown treatment gave an answer")
	}
}
