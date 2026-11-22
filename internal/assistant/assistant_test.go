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
	"github.com/lieranderl/aestheticlab/internal/usage"
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
			ServiceGroups: []content.ServiceGroup{{ID: "1", Name: "Pedicure", NameEN: "Pedicure", AssistantNote: "Book 90 minutes for gel."}},
			Services: []content.Service{
				{ID: "4847871", GroupID: "1", Name: "Pedicure", NameEN: "Pedicure", Description: "Classic pedicure. " + strings.Repeat("Dry technique, no foot bath. ", 20) + "Ends with a light massage.", Duration: 60, Price: 70, StaffIDs: []int64{642297, 642296}, AssistantNote: "Bring open shoes."},
				{ID: "4884910", GroupID: "1", Name: "Coating removal", NameEN: "Coating removal", Duration: 15, Price: 10},
			},
			Staff: []content.Member{
				{ID: 642297, Name: "Zara", Role: "Nail Technician", Schedule: zaraSchedule()},
				{ID: 642296, Name: "Rubina", Role: "Nail Technician"},
			},
			Contact: content.Contact{Email: "studio@example.com"},
			Notes:   []content.Note{{ID: "payment", Topic: "Payment", Text: "Cash or QR code transfer only; no card payments."}},
		},
		Facts:      []string{"Aesthetic Lab, Diestsestraat 174, 3000 Leuven, Belgium"},
		FAQ:        []assistant.QA{{Question: "Can I cancel?", Answer: "Yes, 24 hours ahead."}},
		CareNotes:  "[Laser]\nShave the area 24 hours before your session.",
		Categories: map[string]string{"1": "Thorough, hygienic pedicures."},
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
		"Payment: Cash or QR code transfer only; no card payments.", "Ends with a light massage.", "Shave the area 24 hours before your session.",
		"About this category: Thorough, hygienic pedicures. Book 90 minutes for gel.", "Note: Bring open shoes.",
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

// slotsFor runs find_free_slots with args and returns the times it gave the
// model.
func slotsFor(t *testing.T, args map[string]any) []map[string]any {
	t.Helper()
	m := &scripted{replies: []*genai.GenerateContentResponse{call("find_free_slots", args), text("ok")}}
	if _, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "?"); err != nil {
		t.Fatal(err)
	}
	slots, _ := m.requests[1][2].Parts[0].FunctionResponse.Response["slots"].([]map[string]any)
	return slots
}

// Found on staging: "any day next week at 17:00" got the eight earliest
// times, all on Monday morning, so the assistant said 17:00 was full. A
// time of day narrows the search, and times spread over the days.
func TestFreeTimesCanBeAskedForATimeOfDay(t *testing.T) {
	week := map[string]any{"service_id": "4847871", "from_date": "2026-11-23", "to_date": "2026-11-27"}
	at17 := map[string]any{"from_time": "17:00", "to_time": "17:00"}
	for k, v := range week {
		at17[k] = v
	}
	got := slotsFor(t, at17)
	if len(got) != 5 {
		t.Fatalf("17:00 on five weekdays: got %v", got)
	}
	for _, s := range got {
		if s["time"] != "17:00" {
			t.Errorf("slot %v is not at 17:00", s)
		}
	}

	days := map[any]bool{}
	for _, s := range slotsFor(t, week) {
		days[s["date"]] = true
	}
	if len(days) < 4 {
		t.Errorf("a week without a time gave times on %d days; want them spread over the week", len(days))
	}

	if bad := slotsFor(t, map[string]any{"service_id": "4847871", "from_time": "5pm"}); bad != nil {
		t.Errorf("a malformed time gave slots %v; want an error", bad)
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

// Gemini sometimes writes a note to itself as an ordinary text part before
// the answer: only the last text part is the reply to the visitor.
func TestOnlyTheLastTextPartIsTheReply(t *testing.T) {
	leaky := &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content: &genai.Content{Role: genai.RoleModel, Parts: []*genai.Part{
			{Text: "The user is asking for general beauty advice. I provided a summary, which I have done."},
			{Text: "Вы можете выбрать любую процедуру из нашего списка."},
		}},
		FinishReason: genai.FinishReasonStop,
	}}}
	m := &scripted{replies: []*genai.GenerateContentResponse{leaky}}
	got, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "?")
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "Вы можете выбрать любую процедуру из нашего списка." {
		t.Errorf("reply = %q, want only the answer", got.Text)
	}
}

func TestThePromptForbidsReasoningInTheReply(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{text("ok")}}
	_, _ = newAssistant(m).Reply(context.Background(), knowledge(), nil, "?")
	if prompt := m.configs[0].SystemInstruction.Parts[0].Text; !strings.Contains(prompt, "Write only the message to the visitor") {
		t.Error("the prompt does not forbid notes about reasoning in the reply")
	}
}

// Found by the test set: the assistant said it had "selected 16:00" and
// invented lash-lift aftercare.
func TestThePromptKeepsTimesAndCareAdviceHonest(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{text("ok")}}
	_, _ = newAssistant(m).Reply(context.Background(), knowledge(), nil, "?")
	prompt := m.configs[0].SystemInstruction.Parts[0].Text
	for _, want := range []string{
		"where the visitor chooses the time: never say you booked, reserved or selected a time",
		"mention the Book button only when you called offer_booking",
		"how-we-do-it details only as the information below states them",
		"If a note from the owner and a treatment's description disagree, follow the note",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
}

// The chat shows plain text: Markdown bullets and bold become plain.
func TestMarkdownInTheReplyBecomesPlainText(t *testing.T) {
	md := "Prices:\n\n*   Manicure + gel: **€60**\n- Pedicure: €70\n* Brows: €25\nA*B stays."
	m := &scripted{replies: []*genai.GenerateContentResponse{text(md)}}
	got, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "?")
	if err != nil {
		t.Fatal(err)
	}
	if want := "Prices:\n\n• Manicure + gel: €60\n• Pedicure: €70\n• Brows: €25\nA*B stays."; got.Text != want {
		t.Errorf("reply = %q, want %q", got.Text, want)
	}
}

// textAndCall is a response that writes the answer and calls a tool at once,
// as Gemini often does before showing a Book button.
func textAndCall(text, name string, args map[string]any) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content: &genai.Content{Role: genai.RoleModel, Parts: []*genai.Part{
			{Text: text},
			{FunctionCall: &genai.FunctionCall{ID: "call-1", Name: name, Args: args}},
		}},
		FinishReason: genai.FinishReasonStop,
	}}}
}

// The visitor sees one answer: the model's last, or the one it wrote with
// a Book button when it then says nothing or only adds a closing line.
func TestAnAnswerWrittenWithAToolCallIsKept(t *testing.T) {
	const times = "Zara is free on Monday at 12:15, 12:30 and 12:45, and on Tuesday at 10:00. These times look free now; the booking page confirms them."
	const rewritten = "Zara has free times on Monday at 12:15, 12:30 and 12:45, and on Tuesday at 10:00. The booking page confirms them when you tap Book."
	book := map[string]any{"service_id": "4847871"}
	empty := &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Content: &genai.Content{Role: genai.RoleModel}, FinishReason: genai.FinishReasonStop}}}
	for name, tc := range map[string]struct {
		final *genai.GenerateContentResponse
		want  string
	}{
		"empty final":       {empty, times},
		"no content":        {&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{FinishReason: genai.FinishReasonStop}}}, times},
		"closing line":      {text("Tap Book to choose."), times + "\n\nTap Book to choose."},
		"repeated answer":   {text(times + "\nTap the button to book."), times + "\nTap the button to book."},
		"rewritten answer":  {text(rewritten), rewritten},
		"answer said twice": {text(times), times},
		"a second thought":  {text("I can also look up her other free days."), times + "\n\nI can also look up her other free days."},
		"a disclaimer":      {text("These times look free now."), times + "\n\nThese times look free now."},
	} {
		m := &scripted{replies: []*genai.GenerateContentResponse{textAndCall(times, "offer_booking", book), tc.final}}
		got, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "?")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.Text != tc.want || len(got.Offers) != 1 {
			t.Errorf("%s: reply = %q (%d offers), want %q", name, got.Text, len(got.Offers), tc.want)
		}
	}
}

// Answers written with separate tool calls (one per treatment) are all
// kept when the final round adds nothing.
func TestAnswersFromSeveralToolRoundsAreAllKept(t *testing.T) {
	empty := &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Content: &genai.Content{Role: genai.RoleModel}, FinishReason: genai.FinishReasonStop}}}
	m := &scripted{replies: []*genai.GenerateContentResponse{
		textAndCall("Pedicure costs 70 euros.", "offer_booking", map[string]any{"service_id": "4847871"}),
		textAndCall("Coating removal costs 10 euros.", "offer_booking", map[string]any{"service_id": "4884910"}),
		empty,
	}}
	got, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "?")
	if err != nil {
		t.Fatal(err)
	}
	if want := "Pedicure costs 70 euros.\n\nCoating removal costs 10 euros."; got.Text != want {
		t.Errorf("reply = %q, want %q", got.Text, want)
	}
}

// A reply that ran out of output tokens (Gemini's thinking counts too)
// ends at its last whole sentence, never mid-word.
func TestAReplyCutByTheTokenLimitEndsAtASentence(t *testing.T) {
	cut := text("Ламинирование бровей держится до 3 месяцев. Процедура фиксирует форму волосков, делая брови более густыми и ухожен")
	cut.Candidates[0].FinishReason = genai.FinishReasonMaxTokens
	m := &scripted{replies: []*genai.GenerateContentResponse{cut}}
	got, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "?")
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "Ламинирование бровей держится до 3 месяцев." {
		t.Errorf("reply = %q, want only the whole sentence", got.Text)
	}
	if m.configs[0].MaxOutputTokens < 2048 {
		t.Errorf("max output tokens = %d; thinking and a Russian answer need more than 1024", m.configs[0].MaxOutputTokens)
	}
}

// Text written with a find_free_slots call comes before the times are
// known, so any times in it are guesses: never shown.
func TestTextWrittenBeforeFreeTimesAreKnownIsDropped(t *testing.T) {
	const guess = "Rubina is free on Monday at 10:00, 11:45 and 13:30."
	find := map[string]any{"service_id": "4847871"}
	m := &scripted{replies: []*genai.GenerateContentResponse{
		textAndCall(guess, "find_free_slots", find),
		text("Zara is free on Monday at 10:00."),
	}}
	got, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "?")
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "Zara is free on Monday at 10:00." {
		t.Errorf("reply = %q, want only the answer written after the lookup", got.Text)
	}

	empty := &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Content: &genai.Content{Role: genai.RoleModel}, FinishReason: genai.FinishReasonStop}}}
	m = &scripted{replies: []*genai.GenerateContentResponse{textAndCall(guess, "find_free_slots", find), empty}}
	if got, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "?"); !errors.Is(err, assistant.ErrNoAnswer) {
		t.Errorf("only a guess = %q, %v; want ErrNoAnswer", got.Text, err)
	}
}

func withUsage(r *genai.GenerateContentResponse, prompt, cached, out, thoughts int32) *genai.GenerateContentResponse {
	r.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount: prompt, CachedContentTokenCount: cached, CandidatesTokenCount: out, ThoughtsTokenCount: thoughts,
	}
	return r
}

// Every model call in a reply is billed, thinking as output, so the reply
// carries the sum, failed replies included.
func TestARepliesTokensAreSummedOverItsModelCalls(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{
		withUsage(call("offer_booking", map[string]any{"service_id": "4847871"}), 9000, 8000, 40, 100),
		withUsage(text("Tap the button."), 9200, 8000, 20, 30),
	}}
	got, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "?")
	if err != nil {
		t.Fatal(err)
	}
	if want := (usage.Tokens{Prompt: 18200, Cached: 16000, Output: 190}); got.Tokens != want {
		t.Errorf("tokens = %+v, want %+v", got.Tokens, want)
	}

	blocked := withUsage(&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{FinishReason: genai.FinishReasonSafety}}}, 9000, 0, 0, 50)
	m = &scripted{replies: []*genai.GenerateContentResponse{blocked}}
	got, err = newAssistant(m).Reply(context.Background(), knowledge(), nil, "?")
	if !errors.Is(err, assistant.ErrNoAnswer) || got.Tokens.Prompt != 9000 || got.Tokens.Output != 50 {
		t.Errorf("a blocked reply = %+v, %v; want its tokens with ErrNoAnswer", got.Tokens, err)
	}
}
