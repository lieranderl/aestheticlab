package assistant

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/lieranderl/aestheticlab/internal/usage"
)

// FreeTimes are the free start times find_free_slots found for one
// treatment, exactly as it returned them to the model. The chat shows
// them itself, so a correct lookup can't become a wrong offer in the
// model's prose.
type FreeTimes struct {
	ServiceID string
	Service   string
	// StaffID and Staff are the person the lookup was for; zero for anyone.
	StaffID int64
	Staff   string
	Slots   []Slot
	Failed  bool
	// Unchecked are the people whose times could not be checked.
	Unchecked []string
}

// Slot is a free start time and the person free then.
type Slot struct {
	Start   time.Time
	StaffID int64
	Staff   string
}

// clock is a time of day as the five languages write it: 10:00, 10h00
// (French), 10u00 (Dutch).
var clock = regexp.MustCompile(`(\d{1,2})[:hu]([0-5]\d)`)

// clockTimes are the times of day in s, as HH:MM. A number glued to a
// digit, a decimal point or a currency sign (€12.50, 123:45) is no time of
// day, nor is anything before 06:00, when the studio is long closed: "1h30"
// is a duration.
func clockTimes(s string) []string {
	var out []string
	for _, m := range clock.FindAllStringSubmatchIndex(s, -1) {
		if before, _ := utf8.DecodeLastRuneInString(s[:m[0]]); unicode.IsDigit(before) || strings.ContainsRune(".,€:", before) {
			continue
		}
		if after, _ := utf8.DecodeRuneInString(s[m[1]:]); unicode.IsDigit(after) {
			continue
		}
		h, _ := strconv.Atoi(s[m[2]:m[3]])
		if h < 6 || h > 23 {
			continue
		}
		out = append(out, fmt.Sprintf("%02d:%s", h, s[m[4]:m[5]]))
	}
	return out
}

// showTimes replaces a refinement of the same search and keeps independent
// searches for different staff. Switching to or from all staff supersedes
// earlier results for that treatment with the other staff scope.
func (r *toolRun) showTimes(ft FreeTimes) {
	r.times = slices.DeleteFunc(r.times, func(t FreeTimes) bool {
		return t.ServiceID == ft.ServiceID && t.StaffID != ft.StaffID && (t.StaffID == 0 || ft.StaffID == 0)
	})
	if i := slices.IndexFunc(r.times, func(t FreeTimes) bool {
		return t.ServiceID == ft.ServiceID && t.StaffID == ft.StaffID
	}); i >= 0 {
		r.times[i] = ft
		return
	}
	if len(r.times) < maxOffers {
		r.times = append(r.times, ft)
	}
}

// answer uses the recorded lookup outcomes as the entire availability reply.
// Clock text cannot validate the date, staff, or whether a slot is free.
func (r *toolRun) answer(said []string, known string, tokens usage.Tokens, unmetered int) (Answer, error) {
	var text string
	if len(r.times) == 0 {
		trusted := map[string]bool{}
		for _, c := range clockTimes(known) {
			trusted[c] = true
		}
		checked := make([]string, 0, len(said))
		for _, t := range said {
			if !slices.ContainsFunc(clockTimes(t), func(c string) bool { return !trusted[c] }) {
				checked = append(checked, t)
			}
		}
		text = finalAnswer(checked)
	}
	if text == "" && len(r.times) == 0 {
		return Answer{Tokens: tokens, Unmetered: unmetered}, ErrNoAnswer
	}
	var offers []Offer
	for _, ft := range r.times {
		offers = append(offers, Offer{ServiceID: ft.ServiceID, Service: ft.Service, StaffID: ft.StaffID, Staff: ft.Staff})
	}
	for _, offer := range r.offers {
		if len(offers) < maxOffers && !slices.Contains(offers, offer) {
			offers = append(offers, offer)
		}
	}
	return Answer{Text: text, Offers: offers, Times: r.times, Tokens: tokens, Unmetered: unmetered}, nil
}
