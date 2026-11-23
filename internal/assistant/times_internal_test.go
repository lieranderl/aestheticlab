package assistant

import (
	"slices"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/content"
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

// Gemini's own answers to "When can I book a manicure?", with the
// studio's manicures: it lists the options and asks which, and the chat
// shows that. A sentence that says one can be had is still withheld.
func TestAListOfTheOptionsIsAQuestionBack(t *testing.T) {
	var services []content.Service
	for _, n := range []string{"Manicure", "Manicure+polish", "Manicure + gel polish color (short nails)", "Manicure + gel polish nude (long nails)", "Express manicure + polish", "Male manicure", "Extensions using dual forms"} {
		services = append(services, content.Service{Name: n, NameEN: n})
	}
	cat := newCatalog(Knowledge{Content: &content.Localized{
		ServiceGroups: []content.ServiceGroup{{Name: "Manicure", NameEN: "Manicure"}},
		Services:      services,
		Staff:         []content.Member{{Name: "Zara"}},
	}})
	for reply, want := range map[string]bool{
		"We offer several manicure services: Classic, Manicure + gel polish (nude or color, for short or long nails), Manicure + polish, Express manicure, Male manicure, or Extensions. Which of these would you like to book?": true,
		"We have several types of manicures available: classic, express, or with gel polish (nude, color, long or short nails, or for men). Which one would you like to book?":                                                   true,
		"Which manicure would you like to book? We offer a classic dry manicure, male manicure, express manicure with polish, or various options with gel polish (nude or color, for short or long nails).":                      true,
		"A manicure is available.":                         false,
		"Manicures are available for you.":                 false,
		"We can accommodate you for a manicure.":           false,
		"Several manicure services are possible tomorrow.": false,
		"Zara offers several manicure services.":           false,
	} {
		if got := asksBack(reply, cat); got != want {
			t.Errorf("asksBack(%q) = %v, want %v", reply, got, want)
		}
	}
}
