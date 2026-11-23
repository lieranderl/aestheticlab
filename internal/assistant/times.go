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
	Price     *float64
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
	if ft.Price == nil {
		if i := slices.IndexFunc(r.times, func(t FreeTimes) bool { return t.ServiceID == ft.ServiceID && t.Price != nil }); i >= 0 {
			ft.Price = r.times[i].Price
		}
	}
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

// Appointment phrases in the site's supported languages. Numeric clock times
// in generated prose are handled separately from these common expressions.
var appointmentClaim = regexp.MustCompile(`(?i)\b(?:free slot|free times? (?:are|on|at)|slots? available|appointments? available|no availability|vrij op|vrije (?:plek|tijd|uren)|beschikbaar op|créneaux? (?:libres?|disponibles?)|[0-9]{1,2}\s*(?:a\.?m\.?|p\.?m\.?))\b|свободн(?:ое время|ые окна|а в)|есть окошко|вільн(?:ий час|і години|а в)`)

// "Is free" and "is available" also describe a free consultation or a
// treatment the studio offers. They claim an appointment only in a sentence
// that also says when.
var (
	freeOrAvailable = regexp.MustCompile(`(?i)\b(?:is|are) (?:free|available)\b|\b(?:est|sont) (?:libres?|disponibles?)\b`)
	when            = regexp.MustCompile(`(?i)` +
		`\b(?:mon|tues|wednes|thurs|fri|satur|sun)day|\b(?:today|tomorrow|tonight|this week|next week|weekend|morning|afternoon|evening|(?:at|from|after|before) [0-9])\b` +
		`|\b(?:lundi|mardi|mercredi|jeudi|vendredi|samedi|dimanche|demain|aujourd'hui|matin|soir|semaine|week-end)\b|après-midi|\bà [0-9]` +
		`|\b(?:maandag|dinsdag|woensdag|donderdag|vrijdag|zaterdag|zondag|vandaag|morgen|overmorgen|ochtend|voormiddag|middag|namiddag|avond|deze week|volgende week|om [0-9])` +
		`|понедельник|вторник|среду|среда|четверг|пятниц|суббот|воскресень|сегодня|завтра|утром|утра|днём|днем|вечер|этой неделе|следующей неделе|выходн` +
		`|понеділ|вівтор|середу|середа|четвер|п['’ʼ]?ятниц|субот|неділ|сьогодні|вранці|вдень|ввечері|цього тижня|наступного тижня|вихідн`)
	sentence = regexp.MustCompile(`[.!?;\n]+`)
)

// claimsAvailability reports whether t says when an appointment is free.
func claimsAvailability(t string) bool {
	if len(clockTimes(t)) > 0 || appointmentClaim.MatchString(t) {
		return true
	}
	return slices.ContainsFunc(sentence.Split(t, -1), func(s string) bool {
		return freeOrAvailable.MatchString(s) && when.MatchString(s)
	})
}

// askedWhen are the visitor's words for free times beyond a day or a time.
var askedWhen = regexp.MustCompile(`(?i)availab|\bfree (?:time|slot|day|spot)|\bslots?\b|\bwhen (?:can|could|are|is)\b` +
	`|beschikba|\bvrije? (?:plek|tijd|moment)|\bwanneer\b` +
	`|disponib|créneau|\blibres?\b|\bquand\b` +
	`|свобод|окошк|окно|окна|когда` +
	`|вільн|віконц|коли`)

// asksWhen reports whether the visitor's new message, or the one before it
// (they may name the treatment only once asked), asks when they can come.
func asksWhen(history []Turn, message string) bool {
	said := []string{message}
	for _, t := range slices.Backward(history) {
		if t.Role == RoleVisitor {
			said = append(said, t.Text)
			break
		}
	}
	return slices.ContainsFunc(said, func(s string) bool {
		return len(clockTimes(s)) > 0 || when.MatchString(s) || askedWhen.MatchString(s)
	})
}

// openingWords tell a sentence about the studio's opening days and hours.
var openingWords = regexp.MustCompile(`(?i)\b(?:open|opens|opened|closed?|opening hours|hours)\b|gesloten|openingsuren|openingstijden|ouvert|fermé|horaires|открыт|закрыт|работаем|часы работы|відчинен|зачинен|працюємо|години роботи`)

// namesWhen reports whether t, asked when the visitor can come, names a
// day or a part of the day other than in a sentence about opening hours:
// without a lookup, whatever it says about that day is unchecked.
func namesWhen(t string) bool {
	return slices.ContainsFunc(sentence.Split(t, -1), func(s string) bool {
		return when.MatchString(s) && !openingWords.MatchString(s) && !strings.Contains(s, hoursToken)
	})
}

// hoursToken is what the model writes for the published opening hours.
const hoursToken = "{opening_hours}"

// answer uses the recorded lookup outcomes as the entire availability reply.
// Without a lookup, text that says when an appointment is free is withheld,
// and the visitor told the chat confirms free times only from the calendar.
// Published studio facts can be quoted verbatim; the opening hours are
// filled in from the studio's data.
func (r *toolRun) answer(said []string, tokens usage.Tokens, unmetered int) (Answer, error) {
	var text string
	unverified := false
	if len(r.times) == 0 {
		checked := make([]string, 0, len(said))
		for _, t := range said {
			switch {
			case t == "":
			case slices.Contains(r.k.Facts, t) || t == r.k.Hours:
				checked = append(checked, t)
			case claimsAvailability(t) || r.asksWhen && namesWhen(t):
				unverified = true
			default:
				checked = append(checked, t)
			}
		}
		text = strings.ReplaceAll(finalAnswer(checked), hoursToken, r.k.Hours)
	}
	if text == "" && len(r.times) == 0 && !unverified {
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
	return Answer{Text: text, Offers: offers, Times: r.times, Tokens: tokens, Unmetered: unmetered, Unverified: unverified}, nil
}
