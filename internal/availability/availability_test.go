package availability_test

import (
	"slices"
	"testing"
	"time"

	"github.com/lieranderl/aestheticlab/internal/availability"
)

var brussels = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Brussels")
	if err != nil {
		panic(err)
	}
	return loc
}()

func clock(t *testing.T, s string) availability.Clock {
	t.Helper()
	c, ok := availability.ParseClock(s)
	if !ok {
		t.Fatalf("ParseClock(%q) failed", s)
	}
	return c
}

func shift(t *testing.T, from, to string) availability.Shift {
	t.Helper()
	return availability.Shift{From: clock(t, from), To: clock(t, to)}
}

// at is hh:mm on day in Brussels.
func at(day availability.Date, hh, mm int) time.Time {
	return time.Date(day.Year, day.Month, day.Day, hh, mm, 0, 0, brussels)
}

func hhmm(times []time.Time) []string {
	out := make([]string, len(times))
	for i, t := range times {
		out[i] = t.In(brussels).Format("15:04")
	}
	return out
}

// zara works Monday to Friday 10:00–18:30, bookable every 15 minutes.
func zara(t *testing.T) availability.Schedule {
	t.Helper()
	var w availability.Week
	for d := time.Monday; d <= time.Friday; d++ {
		w[d] = []availability.Shift{shift(t, "10:00", "18:30")}
	}
	return availability.Schedule{Week: w, Step: 15 * time.Minute}
}

var monday = availability.Date{Year: 2026, Month: time.November, Day: 23}

func TestClocksParseOnlyRealTimesOfDay(t *testing.T) {
	for s, want := range map[string]availability.Clock{"00:00": 0, "09:05": 545, "18:30": 1110, "24:00": 1440} {
		if got, ok := availability.ParseClock(s); !ok || got != want {
			t.Errorf("ParseClock(%q) = %d, %v; want %d", s, got, ok, want)
		}
	}
	for _, s := range []string{"", "9:00", "24:01", "12:60", "12-00", "ab:cd", "12:00:00", " 12:00"} {
		if _, ok := availability.ParseClock(s); ok {
			t.Errorf("ParseClock(%q) succeeded", s)
		}
	}
	if got := availability.Clock(1110).String(); got != "18:30" {
		t.Errorf("String() = %q", got)
	}
}

func TestDatesParseAndStep(t *testing.T) {
	d, ok := availability.ParseDate("2026-12-31")
	if !ok || d.String() != "2026-12-31" {
		t.Fatalf("ParseDate = %v, %v", d, ok)
	}
	if got := d.AddDays(1).String(); got != "2027-01-01" {
		t.Errorf("AddDays(1) = %s", got)
	}
	for _, s := range []string{"2026-02-30", "2026-1-01", "31-12-2026", ""} {
		if _, ok := availability.ParseDate(s); ok {
			t.Errorf("ParseDate(%q) succeeded", s)
		}
	}
	if got := availability.DateOf(time.Date(2026, 11, 22, 23, 30, 0, 0, time.UTC), brussels); got.String() != "2026-11-23" {
		t.Errorf("DateOf(23:30 UTC) = %s, want the Brussels date", got)
	}
}

func TestAFreeDayOffersEveryStartThatEndsByTheShiftEnd(t *testing.T) {
	got := zara(t).Starts(monday, brussels, 60*time.Minute, nil, time.Time{})
	if len(got) != 31 {
		t.Fatalf("%d starts, want 31 (10:00 to 17:30 every 15 minutes)", len(got))
	}
	if first, last := hhmm(got)[0], hhmm(got)[30]; first != "10:00" || last != "17:30" {
		t.Errorf("starts run %s–%s, want 10:00–17:30", first, last)
	}
}

func TestBusyTimeRemovesOverlappingStarts(t *testing.T) {
	busy := []availability.Interval{
		{Start: at(monday, 11, 0), End: at(monday, 15, 0)},   // a long booking
		{Start: at(monday, 16, 30), End: at(monday, 18, 30)}, // the rest of the day
	}
	got := hhmm(zara(t).Starts(monday, brussels, 60*time.Minute, busy, time.Time{}))
	// Busy times that reproduce what Timely offered for a 60-minute pedicure
	// with Zara on this day.
	want := []string{"10:00", "15:00", "15:15", "15:30"}
	if !slices.Equal(got, want) {
		t.Errorf("starts = %v, want %v", got, want)
	}
}

func TestATreatmentMayEndExactlyWhenTheNextBookingStarts(t *testing.T) {
	busy := []availability.Interval{{Start: at(monday, 11, 0), End: at(monday, 18, 30)}}
	got := hhmm(zara(t).Starts(monday, brussels, 30*time.Minute, busy, time.Time{}))
	if want := []string{"10:00", "10:15", "10:30"}; !slices.Equal(got, want) {
		t.Errorf("starts = %v, want %v", got, want)
	}
}

func TestNothingStartsBeforeTheEarliestTime(t *testing.T) {
	got := hhmm(zara(t).Starts(monday, brussels, 60*time.Minute, nil, at(monday, 17, 5)))
	if want := []string{"17:15", "17:30"}; !slices.Equal(got, want) {
		t.Errorf("starts = %v, want %v", got, want)
	}
}

func TestDaysOffWeekendsAndTooLongTreatmentsOfferNothing(t *testing.T) {
	s := zara(t)
	s.DaysOff = []availability.Date{monday}
	if got := s.Starts(monday, brussels, time.Hour, nil, time.Time{}); len(got) != 0 {
		t.Errorf("a day off offers %v", hhmm(got))
	}
	saturday := monday.AddDays(5)
	if got := zara(t).Starts(saturday, brussels, time.Hour, nil, time.Time{}); len(got) != 0 {
		t.Errorf("a day without hours offers %v", hhmm(got))
	}
	if got := zara(t).Starts(monday, brussels, 9*time.Hour, nil, time.Time{}); len(got) != 0 {
		t.Errorf("a treatment longer than the shift offers %v", hhmm(got))
	}
	if got := zara(t).Starts(monday, brussels, 0, nil, time.Time{}); len(got) != 0 {
		t.Errorf("a zero-length treatment offers %v", hhmm(got))
	}
}

// Without "Reset the online booking time after a break", Timely keeps the
// day's grid across a break: starts stay on the first shift's quarter hours.
func TestTheGridRunsFromTheDaysFirstShiftAcrossBreaks(t *testing.T) {
	var w availability.Week
	w[time.Monday] = []availability.Shift{shift(t, "10:00", "11:00"), shift(t, "12:10", "13:00")}
	got := hhmm(availability.Schedule{Week: w, Step: 15 * time.Minute}.Starts(monday, brussels, 30*time.Minute, nil, time.Time{}))
	if want := []string{"10:00", "10:15", "10:30", "12:15", "12:30"}; !slices.Equal(got, want) {
		t.Errorf("starts = %v, want %v", got, want)
	}
}

// On the last Sunday of March, Brussels moves from UTC+1 to UTC+2: a 10:00
// start is 08:00 UTC that day and 09:00 UTC the week before.
func TestStartsAreLocalTimesAcrossDaylightSavingChanges(t *testing.T) {
	var w availability.Week
	w[time.Sunday] = []availability.Shift{shift(t, "10:00", "11:00")}
	s := availability.Schedule{Week: w, Step: 30 * time.Minute}
	for day, wantUTC := range map[string]string{"2026-03-22": "09:00", "2026-03-29": "08:00"} {
		d, _ := availability.ParseDate(day)
		got := s.Starts(d, brussels, 30*time.Minute, nil, time.Time{})
		if len(got) != 2 || got[0].UTC().Format("15:04") != wantUTC || got[0].In(brussels).Format("15:04") != "10:00" {
			t.Errorf("%s: starts %v, want 10:00 local = %s UTC", day, got, wantUTC)
		}
	}
}

func TestSchedulesAreValidatedBeforeUse(t *testing.T) {
	valid := zara(t)
	if !valid.Valid() {
		t.Fatal("Zara's schedule is invalid")
	}
	overlapping := zara(t)
	overlapping.Week[time.Monday] = []availability.Shift{shift(t, "10:00", "14:00"), shift(t, "13:00", "18:00")}
	backwards := zara(t)
	backwards.Week[time.Tuesday] = []availability.Shift{shift(t, "18:00", "10:00")}
	noStep := zara(t)
	noStep.Step = 0
	for name, s := range map[string]availability.Schedule{"overlapping": overlapping, "backwards": backwards, "no step": noStep, "empty": {Step: time.Minute}} {
		if s.Valid() {
			t.Errorf("%s schedule is valid", name)
		}
		if got := s.Starts(monday, brussels, time.Hour, nil, time.Time{}); len(got) != 0 {
			t.Errorf("%s schedule offers %v", name, hhmm(got))
		}
	}
}
