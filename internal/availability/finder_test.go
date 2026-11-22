package availability_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/option"

	"github.com/lieranderl/aestheticlab/internal/availability"
)

// fakeSource answers with fixed busy times; calendars in down are missing
// from its answer, as a lost share is.
type fakeSource struct {
	busy  map[string][]availability.Interval
	down  []string
	err   error
	asked []string
}

func (f *fakeSource) Busy(_ context.Context, calendars []string, _, _ time.Time) (map[string][]availability.Interval, error) {
	f.asked = append(f.asked, calendars...)
	if f.err != nil {
		return nil, f.err
	}
	out := map[string][]availability.Interval{}
	for _, c := range calendars {
		if !slices.Contains(f.down, c) {
			out[c] = f.busy[c]
		}
	}
	return out, nil
}

func TestCalendarConfigMapsCalendarsToStaff(t *testing.T) {
	got, err := availability.ParseCalendars(" zara@group.calendar.google.com|642297 ; rubina@group.calendar.google.com|642296;")
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]string{642297: "zara@group.calendar.google.com", 642296: "rubina@group.calendar.google.com"}
	if len(got) != 2 || got[642297] != want[642297] || got[642296] != want[642296] {
		t.Errorf("ParseCalendars = %v, want %v", got, want)
	}
	for _, bad := range []string{"secretcal", "secretcal|x", "|1", "secretcal|1;othercal|1", "secretcal|1;secretcal|2"} {
		_, err := availability.ParseCalendars(bad)
		if err == nil {
			t.Errorf("ParseCalendars(%q) accepted", bad)
			continue
		}
		// The error is logged: it names the pair's position, never a calendar ID.
		if strings.Contains(err.Error(), "secretcal") || strings.Contains(err.Error(), "othercal") {
			t.Errorf("ParseCalendars(%q) error %q shows a calendar ID", bad, err)
		}
	}
	if got, err := availability.ParseCalendars(""); err != nil || len(got) != 0 {
		t.Errorf("empty config = %v, %v", got, err)
	}
}

func finder(t *testing.T, src availability.Source, now time.Time) *availability.Finder {
	t.Helper()
	return availability.NewFinder(availability.FinderOptions{
		Source:    src,
		Calendars: map[int64]string{642297: "zara", 642296: "rubina"},
		Location:  brussels,
		Now:       func() time.Time { return now },
		Lead:      2 * time.Hour,
	})
}

func rubina(t *testing.T) availability.Schedule {
	t.Helper()
	var w availability.Week
	for d := time.Monday; d <= time.Friday; d++ {
		w[d] = []availability.Shift{shift(t, "12:00", "19:00")}
	}
	return availability.Schedule{Week: w, Step: 15 * time.Minute}
}

func TestTheFinderMergesStaffSlotsInTimeOrder(t *testing.T) {
	src := &fakeSource{busy: map[string][]availability.Interval{
		"zara":   {{Start: at(monday, 10, 30), End: at(monday, 18, 30)}},
		"rubina": {{Start: at(monday, 12, 30), End: at(monday, 19, 0)}},
	}}
	z, r := zara(t), rubina(t)
	staff := []availability.Person{{ID: 642297, Name: "Zara", Schedule: &z}, {ID: 642296, Name: "Rubina", Schedule: &r}}
	got, err := finder(t, src, at(monday.AddDays(-1), 20, 0)).Find(context.Background(), staff, 30*time.Minute, monday, monday, 10)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, s := range got.Slots {
		lines = append(lines, s.Start.In(brussels).Format("15:04")+" "+s.Name)
	}
	if want := []string{"10:00 Zara", "12:00 Rubina"}; !slices.Equal(lines, want) {
		t.Errorf("slots = %v, want %v", lines, want)
	}
}

func TestTheFinderLeavesOutUnreadableCalendarsNeverOffersThemFree(t *testing.T) {
	src := &fakeSource{down: []string{"rubina"}}
	z, r := zara(t), rubina(t)
	staff := []availability.Person{{ID: 642297, Name: "Zara", Schedule: &z}, {ID: 642296, Name: "Rubina", Schedule: &r}}
	got, err := finder(t, src, at(monday.AddDays(-1), 20, 0)).Find(context.Background(), staff, time.Hour, monday, monday, 50)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Unavailable, []string{"Rubina"}) {
		t.Errorf("unavailable = %v, want Rubina", got.Unavailable)
	}
	for _, s := range got.Slots {
		if s.StaffID != 642297 {
			t.Fatalf("offered %+v from an unreadable calendar", s)
		}
	}
}

func TestStaffWithoutScheduleOrCalendarAreNotAsked(t *testing.T) {
	src := &fakeSource{}
	z := zara(t)
	staff := []availability.Person{{ID: 642297, Name: "Zara", Schedule: &z}, {ID: 642296, Name: "Rubina"}, {ID: 1, Name: "Nobody", Schedule: &z}}
	got, err := finder(t, src, at(monday.AddDays(-1), 20, 0)).Find(context.Background(), staff, time.Hour, monday, monday, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(src.asked, []string{"zara"}) {
		t.Errorf("asked calendars %v, want only zara", src.asked)
	}
	if len(got.Slots) != 3 {
		t.Errorf("%d slots, want the limit of 3", len(got.Slots))
	}
	if !slices.Equal(got.Unavailable, []string{"Nobody", "Rubina"}) {
		t.Errorf("unavailable = %v", got.Unavailable)
	}
}

func TestTheFinderRespectsLeadTimeAndRange(t *testing.T) {
	z := zara(t)
	staff := []availability.Person{{ID: 642297, Name: "Zara", Schedule: &z}}
	// Monday 16:00: the 2-hour lead leaves 18:00 and later, too late for an hour.
	got, err := finder(t, &fakeSource{}, at(monday, 16, 0)).Find(context.Background(), staff, time.Hour, monday, monday.AddDays(1), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Slots) == 0 || availability.DateOf(got.Slots[0].Start, brussels) != monday.AddDays(1) {
		t.Errorf("first slot %v, want Tuesday", got.Slots)
	}
	if _, err := finder(t, &fakeSource{}, at(monday, 9, 0)).Find(context.Background(), staff, time.Hour, monday, monday.AddDays(15), 5); !errors.Is(err, availability.ErrRange) {
		t.Errorf("a 16-day range: err = %v, want ErrRange", err)
	}
	if _, err := finder(t, &fakeSource{}, at(monday, 9, 0)).Find(context.Background(), staff, time.Hour, monday.AddDays(2), monday, 5); !errors.Is(err, availability.ErrRange) {
		t.Errorf("a backwards range: err = %v, want ErrRange", err)
	}
}

func TestACalendarOutageIsAnError(t *testing.T) {
	z := zara(t)
	staff := []availability.Person{{ID: 642297, Name: "Zara", Schedule: &z}}
	_, err := finder(t, &fakeSource{err: errors.New("boom")}, at(monday, 9, 0)).Find(context.Background(), staff, time.Hour, monday, monday, 5)
	if !errors.Is(err, availability.ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}

// The Google source asks freeBusy for every calendar at once and drops any
// calendar the answer marks with errors.
func TestGoogleFreeBusyDropsCalendarsWithErrors(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/freeBusy") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"calendars":{
			"zara":{"busy":[{"start":"2026-11-23T10:00:00Z","end":"2026-11-23T11:00:00Z"}]},
			"rubina":{"errors":[{"domain":"global","reason":"notFound"}]}}}`))
	}))
	defer srv.Close()
	src, err := availability.NewGoogle(context.Background(), option.WithEndpoint(srv.URL+"/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	got, err := src.Busy(context.Background(), []string{"zara", "rubina"}, at(monday, 0, 0), at(monday.AddDays(1), 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["rubina"]; ok {
		t.Error("a calendar with errors was answered as free")
	}
	if z := got["zara"]; len(z) != 1 || !z[0].Start.Equal(time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("zara = %v", z)
	}
	if items, _ := body["items"].([]any); len(items) != 2 {
		t.Errorf("request items = %v, want both calendars in one call", body["items"])
	}
}

func TestTheDevSourceIsStableAndBusySomeOfTheTime(t *testing.T) {
	src := availability.DevSource{Location: brussels}
	a, _ := src.Busy(context.Background(), []string{"zara"}, at(monday, 0, 0), at(monday.AddDays(7), 0, 0))
	b, _ := src.Busy(context.Background(), []string{"zara"}, at(monday, 0, 0), at(monday.AddDays(7), 0, 0))
	if len(a["zara"]) == 0 || len(a["zara"]) != len(b["zara"]) {
		t.Errorf("dev busy times = %d then %d, want the same non-empty set", len(a["zara"]), len(b["zara"]))
	}
}

// For local work and the browser tests, made-up hours stand in for staff
// without their own (the content snapshot has none), so the chat can show
// times. Staff with hours keep theirs.
func TestDevHoursStandInForStaffWithoutTheirOwn(t *testing.T) {
	r := rubina(t)
	f := availability.NewFinder(availability.FinderOptions{
		Source: availability.DevSource{Location: brussels}, EveryStaff: true, DevHours: true,
		Location: brussels, Now: func() time.Time { return at(monday.AddDays(-1), 20, 0) },
	})
	staff := []availability.Person{{ID: 642297, Name: "Zara"}, {ID: 642296, Name: "Rubina", Schedule: &r}}
	got, err := f.Find(context.Background(), staff, time.Hour, monday, monday.AddDays(5), 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Unavailable) != 0 {
		t.Errorf("unavailable = %v, want everyone checked", got.Unavailable)
	}
	saturday := map[string]bool{}
	for _, s := range got.Slots {
		at := s.Start.In(brussels)
		if s.StaffID == 642296 && (at.Hour() < 12 || at.Weekday() == time.Saturday) {
			t.Errorf("Rubina offered %v, outside her own hours", at)
		}
		if at.Weekday() == time.Saturday {
			saturday[s.Name] = true
		}
	}
	if !saturday["Zara"] {
		t.Error("Zara has no made-up times on Saturday")
	}
}
