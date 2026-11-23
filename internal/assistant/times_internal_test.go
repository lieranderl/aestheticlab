package assistant

import (
	"slices"
	"testing"
)

// Times of day are read in the ways the five languages write them; prices
// and durations are not times.
func TestClockTimesReadsTimesOfDay(t *testing.T) {
	for in, want := range map[string][]string{
		"Zara is free at 10:00 and 9:15.":    {"10:00", "09:15"},
		"Lundi à 17h00, mardi à 9h30.":       {"17:00", "09:30"},
		"Maandag om 17u00.":                  {"17:00"},
		"Open 10:00 – 18:30.":                {"10:00", "18:30"},
		"It costs €12.50 and takes 1.5 h.":   nil,
		"Ref 123:45 and 24:00 and 10:75.":    nil,
		"В понедельник в 10:00 у Зары.":      {"10:00"},
		"Within 24 hours, 2 months, 30 min.": nil,
		"La séance dure 1h30, ou 2:00.":      nil,
	} {
		if got := clockTimes(in); !slices.Equal(got, want) {
			t.Errorf("clockTimes(%q) = %v, want %v", in, got, want)
		}
	}
}
