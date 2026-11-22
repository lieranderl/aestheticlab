package assistant_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/genai"

	"github.com/lieranderl/aestheticlab/internal/assistant"
	"github.com/lieranderl/aestheticlab/internal/availability"
	"github.com/lieranderl/aestheticlab/internal/content"
	"github.com/lieranderl/aestheticlab/internal/i18n"
)

var brussels = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Brussels")
	if err != nil {
		panic(err)
	}
	return loc
}()

// Sunday 22 November 2026, 20:00 in Brussels.
var now = time.Date(2026, 11, 22, 20, 0, 0, 0, brussels)

// scripted is a model that answers from a list, recording every request.
type scripted struct {
	replies  []*genai.GenerateContentResponse
	requests [][]*genai.Content
	configs  []*genai.GenerateContentConfig
	err      error
}

func (s *scripted) Generate(_ context.Context, contents []*genai.Content, cfg *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	s.requests = append(s.requests, slices.Clone(contents))
	s.configs = append(s.configs, cfg)
	if s.err != nil {
		return nil, s.err
	}
	if len(s.replies) == 0 {
		return nil, errors.New("no scripted reply left")
	}
	r := s.replies[0]
	s.replies = s.replies[1:]
	return r, nil
}

func text(s string) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content: genai.NewContentFromText(s, genai.RoleModel), FinishReason: genai.FinishReasonStop,
	}}}
}

func call(name string, args map[string]any) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content: &genai.Content{Role: genai.RoleModel, Parts: []*genai.Part{
			{FunctionCall: &genai.FunctionCall{ID: "call-1", Name: name, Args: args}, ThoughtSignature: []byte("sig")},
		}},
		FinishReason: genai.FinishReasonStop,
	}}}
}

func zaraSchedule() *availability.Schedule {
	var w availability.Week
	for d := time.Monday; d <= time.Friday; d++ {
		w[d] = []availability.Shift{{From: 600, To: 1110}}
	}
	return &availability.Schedule{Week: w, Step: 15 * time.Minute}
}

func knowledge() assistant.Knowledge {
	return assistant.Knowledge{
		Locale: i18n.EnBE,
		Content: &content.Localized{
			ServiceGroups: []content.ServiceGroup{{ID: "1", Name: "Pedicure", NameEN: "Pedicure"}},
			Services: []content.Service{
				{ID: "4847871", GroupID: "1", Name: "Pedicure", NameEN: "Pedicure", Description: "Classic pedicure.", Duration: 60, Price: 70, StaffIDs: []int64{642297, 642296}},
				{ID: "4884910", GroupID: "1", Name: "Coating removal", NameEN: "Coating removal", Duration: 15, Price: 10},
			},
			Staff: []content.Member{
				{ID: 642297, Name: "Zara", Role: "Nail Technician", Schedule: zaraSchedule()},
				{ID: 642296, Name: "Rubina", Role: "Nail Technician"},
			},
			Contact: content.Contact{Email: "studio@example.com"},
		},
		Facts: []string{"Aesthetic Lab, Diestsestraat 174, 3000 Leuven, Belgium"},
		FAQ:   []assistant.QA{{Question: "Can I cancel?", Answer: "Yes, 24 hours ahead."}},
	}
}

type busySource struct{}

func (busySource) Busy(_ context.Context, calendars []string, _, _ time.Time) (map[string][]availability.Interval, error) {
	out := map[string][]availability.Interval{}
	for _, c := range calendars {
		out[c] = nil
	}
	return out, nil
}

func newAssistant(m assistant.Model) *assistant.Assistant {
	return assistant.New(assistant.Options{
		Model: m,
		Finder: availability.NewFinder(availability.FinderOptions{
			Source: busySource{}, Calendars: map[int64]string{642297: "zara"}, Location: brussels,
			Now: func() time.Time { return now }, Lead: 2 * time.Hour,
		}),
		Location: brussels,
		Now:      func() time.Time { return now },
	})
}

func TestTheSystemPromptCarriesTheSalonAndTheRules(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{text("Hello!")}}
	if _, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "Hi"); err != nil {
		t.Fatal(err)
	}
	cfg := m.configs[0]
	prompt := cfg.SystemInstruction.Parts[0].Text
	for _, want := range []string{
		"only", "Aesthetic Lab", "service_id=4847871", "Pedicure", "€70", "60 min", "Zara", "Rubina",
		"Diestsestraat 174", "Can I cancel?", "medical", "studio@example.com",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	if strings.Contains(prompt, "2026-11-22") {
		t.Error("the date is in the system prompt; it changes daily and would defeat caching")
	}
	names := []string{}
	for _, tool := range cfg.Tools {
		for _, f := range tool.FunctionDeclarations {
			names = append(names, f.Name)
		}
	}
	if !slices.Equal(names, []string{"find_free_slots", "offer_booking"}) {
		t.Errorf("tools = %v", names)
	}
}

func TestTheSystemPromptIsTheSameForEveryRequest(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{text("a"), text("b")}}
	a := newAssistant(m)
	_, _ = a.Reply(context.Background(), knowledge(), nil, "first")
	_, _ = a.Reply(context.Background(), knowledge(), []assistant.Turn{{Role: assistant.RoleVisitor, Text: "first"}, {Role: assistant.RoleAssistant, Text: "a"}}, "second")
	if m.configs[0].SystemInstruction.Parts[0].Text != m.configs[1].SystemInstruction.Parts[0].Text {
		t.Error("the system prompt changed between requests")
	}
}

// The transcript reaches the model as quoted data inside one visitor
// message, with today's date, never as model turns it could be led by.
func TestTheTranscriptIsQuotedDataInOneMessage(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{text("ok")}}
	history := []assistant.Turn{{Role: assistant.RoleVisitor, Text: "Do you do pedicures?"}, {Role: assistant.RoleAssistant, Text: "Yes, €70."}}
	if _, err := newAssistant(m).Reply(context.Background(), knowledge(), history, "Any time Monday?"); err != nil {
		t.Fatal(err)
	}
	contents := m.requests[0]
	if len(contents) != 1 || contents[0].Role != genai.RoleUser {
		t.Fatalf("contents = %d messages, want one user message", len(contents))
	}
	msg := contents[0].Parts[0].Text
	for _, want := range []string{"Do you do pedicures?", "Yes, €70.", "Any time Monday?", "Sunday 2026-11-22"} {
		if !strings.Contains(msg, want) {
			t.Errorf("user message lacks %q:\n%s", want, msg)
		}
	}
}

func TestFindFreeSlotsAnswersFromHoursAndCalendars(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{
		call("find_free_slots", map[string]any{"service_id": "4847871", "from_date": "2026-11-23", "to_date": "2026-11-23"}),
		text("Zara is free at 10:00."),
	}}
	got, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "Pedicure on Monday?")
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "Zara is free at 10:00." {
		t.Errorf("text = %q", got.Text)
	}
	second := m.requests[1]
	if len(second) != 3 || string(second[1].Parts[0].ThoughtSignature) != "sig" {
		t.Fatalf("the model's call was not sent back unchanged: %d messages", len(second))
	}
	resp := second[2].Parts[0].FunctionResponse
	if resp == nil || resp.Name != "find_free_slots" || resp.ID != "call-1" {
		t.Fatalf("function response = %+v", resp)
	}
	slots, _ := resp.Response["slots"].([]map[string]any)
	if len(slots) == 0 || slots[0]["time"] != "10:00" || slots[0]["staff"] != "Zara" || slots[0]["date"] != "2026-11-23" {
		t.Errorf("slots = %v", resp.Response["slots"])
	}
	if unchecked, _ := resp.Response["could_not_check"].([]string); !slices.Equal(unchecked, []string{"Rubina"}) {
		t.Errorf("could_not_check = %v, want Rubina (no hours)", resp.Response["could_not_check"])
	}
}

func TestToolsRejectUnknownServicesAndStaff(t *testing.T) {
	for name, args := range map[string]map[string]any{
		"unknown service":       {"service_id": "999"},
		"staff not on service":  {"service_id": "4847871", "staff_id": float64(672362)},
		"service without staff": {"service_id": "4884910"},
		"bad date":              {"service_id": "4847871", "from_date": "Monday"},
	} {
		m := &scripted{replies: []*genai.GenerateContentResponse{call("find_free_slots", args), text("Sorry.")}}
		if _, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "?"); err != nil {
			t.Fatal(err)
		}
		resp := m.requests[1][2].Parts[0].FunctionResponse.Response
		if _, ok := resp["error"]; !ok {
			t.Errorf("%s: response = %v, want an error", name, resp)
		}
	}
}

func TestOfferBookingBecomesABookButton(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{
		call("offer_booking", map[string]any{"service_id": "4847871", "staff_id": float64(642297)}),
		text("Tap the button to book."),
	}}
	got, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "Book with Zara")
	if err != nil {
		t.Fatal(err)
	}
	want := []assistant.Offer{{ServiceID: "4847871", Service: "Pedicure", StaffID: 642297, Staff: "Zara"}}
	if !slices.Equal(got.Offers, want) {
		t.Errorf("offers = %+v, want %+v", got.Offers, want)
	}
}

func TestTheToolLoopIsBounded(t *testing.T) {
	loop := call("offer_booking", map[string]any{"service_id": "4847871"})
	m := &scripted{replies: []*genai.GenerateContentResponse{loop, loop, loop, loop, loop, loop}}
	if _, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "?"); !errors.Is(err, assistant.ErrNoAnswer) {
		t.Errorf("err = %v, want ErrNoAnswer", err)
	}
	if len(m.requests) != 4 {
		t.Errorf("%d model calls, want 4", len(m.requests))
	}
}

func TestBlockedOrEmptyAnswersAreErrors(t *testing.T) {
	blocked := &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{FinishReason: genai.FinishReasonSafety}}}
	for name, r := range map[string]*genai.GenerateContentResponse{"blocked": blocked, "empty": text("  "), "no candidates": {}} {
		m := &scripted{replies: []*genai.GenerateContentResponse{r}}
		if _, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "?"); !errors.Is(err, assistant.ErrNoAnswer) {
			t.Errorf("%s: err = %v, want ErrNoAnswer", name, err)
		}
	}
}

func TestTheFakeModelLooksUpTimesAndOffersBooking(t *testing.T) {
	a := newAssistant(assistant.Fake{})
	got, err := a.Reply(context.Background(), knowledge(), nil, "When are you free?")
	if err != nil || !strings.Contains(got.Text, "Zara") {
		t.Errorf("times reply = %q, %v", got.Text, err)
	}
	got, err = a.Reply(context.Background(), knowledge(), nil, "I want to book")
	if err != nil || len(got.Offers) != 1 {
		t.Errorf("booking reply = %+v, %v", got, err)
	}
	got, err = a.Reply(context.Background(), knowledge(), nil, "What's the weather?")
	if err != nil || !strings.Contains(got.Text, "Aesthetic Lab") {
		t.Errorf("other reply = %q, %v", got.Text, err)
	}
}
