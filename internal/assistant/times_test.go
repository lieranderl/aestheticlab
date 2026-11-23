package assistant_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"google.golang.org/genai"

	"github.com/lieranderl/aestheticlab/internal/assistant"
	"github.com/lieranderl/aestheticlab/internal/availability"
)

// toolSlots are the free times find_free_slots returned to the model in
// its request i (the response that follows the call).
func toolSlots(t *testing.T, m *scripted, i int) []map[string]any {
	t.Helper()
	contents := m.requests[i]
	resp := contents[len(contents)-1].Parts[0].FunctionResponse
	if resp == nil || resp.Name != "find_free_slots" {
		t.Fatalf("request %d does not answer find_free_slots", i)
	}
	slots, _ := resp.Response["slots"].([]map[string]any)
	return slots
}

// sameSlots reports whether the shown slots are the tool's, in order:
// the same date, time and person.
func sameSlots(shown []assistant.Slot, tool []map[string]any) bool {
	return slices.EqualFunc(shown, tool, func(s assistant.Slot, m map[string]any) bool {
		t := s.Start.In(brussels)
		return t.Format(time.DateOnly) == m["date"] && t.Format("15:04") == m["time"] && s.StaffID == m["staff_id"] && s.Staff == m["staff"]
	})
}

func monday(extra map[string]any) map[string]any {
	args := map[string]any{"service_id": "4847871", "from_date": "2026-11-23", "to_date": "2026-11-23"}
	for k, v := range extra {
		args[k] = v
	}
	return args
}

// The visitor reads free times from the lookup itself, never from the
// model's prose: the answer carries exactly the slots find_free_slots
// returned, with a Book button for the treatment.
func TestTheFreeTimesShownAreTheToolsResult(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{
		call("find_free_slots", monday(nil)),
		text("Zara has a free time on Monday morning."),
	}}
	got, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "Pedicure on Monday?")
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "" {
		t.Errorf("text = %q", got.Text)
	}
	if len(got.Times) != 1 {
		t.Fatalf("times = %+v, want one treatment's", got.Times)
	}
	ft := got.Times[0]
	tool := toolSlots(t, m, 1)
	if ft.ServiceID != "4847871" || ft.Service != "Pedicure" || ft.StaffID != 0 || len(tool) == 0 || !sameSlots(ft.Slots, tool) {
		t.Errorf("shown %+v; the tool returned %v", ft, tool)
	}
	if !slices.Equal(ft.Unchecked, []string{"Rubina"}) {
		t.Errorf("unchecked = %v, want Rubina (no hours)", ft.Unchecked)
	}
	if want := []assistant.Offer{{ServiceID: "4847871", Service: "Pedicure"}}; !slices.Equal(got.Offers, want) {
		t.Errorf("offers = %+v, want %+v: shown times come with a Book button", got.Offers, want)
	}
}

// A second lookup for the same treatment (a narrower hour, one person)
// replaces the first: the visitor sees the times the model last saw.
func TestALaterLookupForATreatmentReplacesTheEarlierOne(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{
		call("find_free_slots", map[string]any{"service_id": "4847871", "from_date": "2026-11-23", "to_date": "2026-11-27"}),
		call("find_free_slots", map[string]any{"service_id": "4847871", "staff_id": float64(642297), "from_date": "2026-11-23", "to_date": "2026-11-27", "from_time": "17:00"}),
		text("Zara works until the evening on weekdays."),
	}}
	got, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "With Zara after 17:00?")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Times) != 1 || !sameSlots(got.Times[0].Slots, toolSlots(t, m, 2)) {
		t.Fatalf("times = %+v; want the second lookup's %v", got.Times, toolSlots(t, m, 2))
	}
	ft := got.Times[0]
	if ft.StaffID != 642297 || ft.Staff != "Zara" || ft.Unchecked != nil {
		t.Errorf("times = %+v, want Zara's", ft)
	}
	for _, s := range ft.Slots {
		if s.Start.In(brussels).Format("15:04") < "17:00" {
			t.Errorf("slot %v is before 17:00", s.Start)
		}
	}
	if want := []assistant.Offer{{ServiceID: "4847871", Service: "Pedicure", StaffID: 642297, Staff: "Zara"}}; !slices.Equal(got.Offers, want) {
		t.Errorf("offers = %+v, want %+v", got.Offers, want)
	}

	// A narrower lookup that finds nothing clears the earlier times: they
	// don't fit what the visitor asked for now.
	m = &scripted{replies: []*genai.GenerateContentResponse{
		call("find_free_slots", monday(nil)),
		call("find_free_slots", monday(map[string]any{"from_time": "18:00"})),
		text("Nothing starts that late on Monday."),
	}}
	got, err = newAssistant(m).Reply(context.Background(), knowledge(), nil, "Monday evening?")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Times) != 1 || len(got.Times[0].Slots) != 0 || got.Text != "" {
		t.Errorf("after an empty lookup: %+v; want only the verified empty result", got)
	}
}

// Availability prose is suppressed even if it repeats a time in the question.
func TestAReplyNamingATimeNotFoundIsDropped(t *testing.T) {
	k := knowledge()
	k.Facts = append(k.Facts, "Monday – Saturday · 10:00 – 18:30 · By appointment only")
	for name, tc := range map[string]struct {
		replies  []*genai.GenerateContentResponse
		message  string
		want     string
		wantErr  error
		hasTimes bool
	}{
		"a shown time": {
			replies:  []*genai.GenerateContentResponse{call("find_free_slots", monday(nil)), text("Zara is free on Monday from 10:00.")},
			want:     "",
			hasTimes: true,
		},
		"an invented time": {
			replies:  []*genai.GenerateContentResponse{call("find_free_slots", monday(nil)), text("Zara is free on Monday at 10:00 and 11h30.")},
			want:     "",
			hasTimes: true,
		},
		"an affirmation of the hour asked after an empty lookup": {
			replies: []*genai.GenerateContentResponse{
				call("find_free_slots", monday(map[string]any{"from_time": "18:00"})),
				text("Yes, Monday at 18:00 is free."),
			},
			message:  "Monday at 18:00?",
			hasTimes: true,
		},
		"an echoed visitor time without a lookup": {
			replies: []*genai.GenerateContentResponse{text("Yes, Zara is free at 10:00.")},
			message: "Can I book Zara at 10:00?",
			wantErr: assistant.ErrNoAnswer,
		},
		"availability claim without a numeric time or lookup": {
			replies: []*genai.GenerateContentResponse{text("Zara is available on Monday.")},
			message: "Is Zara free on Monday?",
			wantErr: assistant.ErrNoAnswer,
		},
		"published opening hours": {
			replies: []*genai.GenerateContentResponse{text("Monday – Saturday · 10:00 – 18:30 · By appointment only")},
			want:    "Monday – Saturday · 10:00 – 18:30 · By appointment only",
		},
		"a time without a lookup": {
			replies: []*genai.GenerateContentResponse{text("Zara is free tomorrow at 14:00.")},
			wantErr: assistant.ErrNoAnswer,
		},
	} {
		m := &scripted{replies: tc.replies}
		got, err := newAssistant(m).Reply(context.Background(), k, nil, tc.message)
		if !errors.Is(err, tc.wantErr) {
			t.Fatalf("%s: err = %v, want %v", name, err, tc.wantErr)
		}
		if got.Text != tc.want || (len(got.Times) > 0) != tc.hasTimes {
			t.Errorf("%s: text %q with %d times; want %q, times %v", name, got.Text, len(got.Times), tc.want, tc.hasTimes)
		}
		if tc.hasTimes && !sameSlots(got.Times[0].Slots, toolSlots(t, m, 1)) {
			t.Errorf("%s: times = %+v, not the tool's", name, got.Times[0].Slots)
		}
	}
}

// A plain availability question needs no model round that only repeats
// the times: with only_times the reply ends with the lookup, its times
// and a Book button. Text written with the call is a guess, never shown.
func TestAPlainAvailabilityQuestionEndsWithTheTimes(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{
		textAndCall("Zara is free at 9:00.", "find_free_slots", monday(map[string]any{"only_times": true})),
	}}
	got, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "When can I come on Monday?")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.requests) != 1 {
		t.Errorf("%d model calls, want 1", len(m.requests))
	}
	if got.Text != "" || len(got.Times) != 1 || len(got.Times[0].Slots) == 0 || len(got.Offers) != 1 {
		t.Errorf("answer = %+v; want only the times and a Book button", got)
	}

	// Without times to show, the model explains.
	m = &scripted{replies: []*genai.GenerateContentResponse{
		call("find_free_slots", monday(map[string]any{"only_times": true, "from_time": "23:00"})),
		text("There are no free times that late."),
	}}
	got, err = newAssistant(m).Reply(context.Background(), knowledge(), nil, "Monday night?")
	if err != nil || len(m.requests) != 2 || got.Text != "" || len(got.Times) != 1 || len(got.Times[0].Slots) != 0 {
		t.Errorf("no times found: %+v, %v after %d model calls; want the model's explanation", got, err, len(m.requests))
	}
}

func TestIndependentStaffLookupsKeepTheirOwnResults(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{
		call("find_free_slots", monday(map[string]any{"staff_id": float64(642297)})),
		call("find_free_slots", monday(map[string]any{"staff_id": float64(642296)})),
		text("They are both free."),
	}}
	got, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "Compare Zara and Rubina")
	if err != nil || got.Text != "" || len(got.Times) != 2 ||
		got.Times[0].Staff != "Zara" || len(got.Times[0].Slots) == 0 ||
		got.Times[1].Staff != "Rubina" || len(got.Times[1].Slots) != 0 || !got.Times[1].Failed {
		t.Fatalf("want Zara's slots and Rubina's unavailable result, got %+v, %v", got, err)
	}
}

type timeoutOnSecondRead struct{ reads int }

func (s *timeoutOnSecondRead) Busy(ctx context.Context, calendars []string, from, to time.Time) (map[string][]availability.Interval, error) {
	s.reads++
	if s.reads == 2 {
		return nil, context.DeadlineExceeded
	}
	return (busySource{}).Busy(ctx, calendars, from, to)
}

func TestFailedRefinementReplacesEarlierSlots(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{
		call("find_free_slots", monday(nil)),
		call("find_free_slots", monday(map[string]any{"from_time": "17:00"})),
		text("The earlier slots are still available."),
	}}
	source := &timeoutOnSecondRead{}
	a := assistant.New(assistant.Options{Model: m, Location: brussels, Now: func() time.Time { return now },
		Finder: availability.NewFinder(availability.FinderOptions{Source: source, Calendars: map[int64]string{642297: "zara"},
			Location: brussels, Now: func() time.Time { return now }}),
	})
	got, err := a.Reply(context.Background(), knowledge(), nil, "Check another window")
	if err != nil || got.Text != "" || len(got.Times) != 1 || len(got.Times[0].Slots) != 0 || !got.Times[0].Failed {
		t.Fatalf("want only the failed refinement, got %+v, %v", got, err)
	}
}

func TestAllStaffRefinementReplacesStaffSpecificResults(t *testing.T) {
	m := &scripted{replies: []*genai.GenerateContentResponse{
		call("find_free_slots", monday(map[string]any{"staff_id": float64(642297)})),
		call("find_free_slots", monday(map[string]any{"staff_id": float64(642296)})),
		call("find_free_slots", monday(map[string]any{"from_time": "23:00"})),
		text("There are no free times that late."),
	}}
	got, err := newAssistant(m).Reply(context.Background(), knowledge(), nil, "Check Zara and Rubina, then anyone after 23:00")
	if err != nil || len(got.Times) != 1 || got.Times[0].StaffID != 0 ||
		len(got.Times[0].Slots) != 0 || got.Times[0].Failed ||
		len(got.Offers) != 1 || got.Offers[0].StaffID != 0 {
		t.Fatalf("want only the empty all-staff result and its booking offer, got %+v, %v", got, err)
	}
}
