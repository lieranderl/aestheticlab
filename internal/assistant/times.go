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

// showTimes keeps ft for the visitor: a later lookup for the same
// treatment replaces an earlier one, and one that found nothing clears it,
// since the earlier times don't fit what was asked now. At most maxOffers
// treatments show times.
func (r *toolRun) showTimes(ft FreeTimes) {
	if len(ft.Slots) == 0 {
		r.times = slices.DeleteFunc(r.times, func(t FreeTimes) bool { return t.ServiceID == ft.ServiceID })
		return
	}
	if i := slices.IndexFunc(r.times, func(t FreeTimes) bool { return t.ServiceID == ft.ServiceID }); i >= 0 {
		r.times[i] = ft
		return
	}
	if len(r.times) < maxOffers {
		r.times = append(r.times, ft)
	}
}

// answer is the reply from the rounds' texts and the tools' results. A
// text that names a time of day the visitor can't trust is dropped: every
// time in it must be a shown free time, one the visitor or a lookup
// named, or one in known (the system prompt and the visitor's message:
// opening hours, the transcript). Shown times come with a Book button for
// their treatment if the model offered none.
func (r *toolRun) answer(said []string, known string, tokens usage.Tokens, unmetered int) (Answer, error) {
	trusted := map[string]bool{}
	for _, c := range append(clockTimes(known), r.asked...) {
		trusted[c] = true
	}
	for _, ft := range r.times {
		for _, s := range ft.Slots {
			trusted[s.Start.In(r.a.opts.Location).Format("15:04")] = true
		}
	}
	checked := make([]string, 0, len(said))
	for _, t := range said {
		if slices.ContainsFunc(clockTimes(t), func(c string) bool { return !trusted[c] }) {
			continue
		}
		checked = append(checked, t)
	}
	text := finalAnswer(checked)
	if text == "" && len(r.times) == 0 {
		return Answer{Tokens: tokens, Unmetered: unmetered}, ErrNoAnswer
	}
	offers := slices.Clone(r.offers)
	for _, ft := range r.times {
		if len(offers) < maxOffers && !slices.ContainsFunc(offers, func(o Offer) bool { return o.ServiceID == ft.ServiceID }) {
			offers = append(offers, Offer{ServiceID: ft.ServiceID, Service: ft.Service, StaffID: ft.StaffID, Staff: ft.Staff})
		}
	}
	return Answer{Text: text, Offers: offers, Times: r.times, Tokens: tokens, Unmetered: unmetered}, nil
}
