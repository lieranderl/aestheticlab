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

	"github.com/lieranderl/aestheticlab/internal/content"
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
		`|\b(?:maandag|dinsdag|woensdag|donderdag|vrijdag|zaterdag|zondag|vandaag|morgen|overmorgen|ochtend|voormiddag|middag|namiddag|avond|vanavond|vanochtend|vanmorgen|vanmiddag|deze week|volgende week|om [0-9])` +
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

// The visitor asks when they can come in two ways. Asking for free times
// (a free slot, a time of day) leaves any day the reply names unchecked.
// Naming a day or a time, or asking when, is taken as asking to come
// unless the message is plainly about something else (aftercare, the
// opening hours, a duration, a price, the address): then the chat has nothing to say
// without a lookup but a question back or the studio's opening hours, and
// a yes, a no or "that works" is unchecked whatever words it uses. When in
// doubt the visitor reads that the chat confirms free times only from the
// calendar, never an unchecked yes.
var (
	freeTimeWords = regexp.MustCompile(`(?i)availab|\bfree (?:time|slot|day|spot)|\bslots?\b|\b(?:are you|anyone|someone) (?:free|available)\b` +
		`|beschikba|\bvrije? (?:plek|tijd|moment)` +
		`|disponib|créneau|\blibres?\b` +
		`|свобод|окошк|окно|окна` +
		`|вільн|віконц`)
	// comeWords ask to come or book, whatever else the message says.
	comeWords = regexp.MustCompile(`(?i)\b(?:i|we|to|i'll) (?:come|visit|drop by|pop in)\b|\b(?:can|could|may) (?:i|we) (?:come|visit|get in)\b|\bsee (?:me|us)\b|\bbook|\bappointment|\breserv` +
		`|\bkomen\b|\blangs ?kom|\bboeken|\bafspraak|\bterecht\b` +
		`|\bvenir\b|\bviens\b|réserv|rendez-vous|\brdv\b` +
		`|прийти|приду|прийду|зайти|запис|на при[её]м|бронир|к вам` +
		`|на прийом|заброн|до вас`)
	// otherTopics are what a message with a day in it asks about when it
	// isn't asking to come: aftercare, the time after a treatment, the
	// opening hours, a duration, a consultation, a price, the address.
	otherTopics = regexp.MustCompile(`(?i)\bafter\b|\bapply|\bwash|\bwear|\buse\b|\bavoid|\bshower|\bbath|\bswim|\bsauna|\bsun\b|\bsunbath|\bsunscreen|\bsunbed|\btan(?:ning)?\b|make-?up|\bcream|\boil\b|\bserum|moistur|\bexercis|\bsport|\bgym\b|\bsweat|\bshav|\bpluck|\btweez|\bscratch|\bwet\b|\bheal|\bredness|\bred\b|\bswell|\bitch|irritat|\bsore|\bpain|\bburn|\bbruis|\bfade|\bscab|\bpeel|regrow|grow back|\bresults?\b|aftercare|skincare|\bcare (?:for|of)` +
		`|\bopen\b|\bclosed\b|\bopening|\bhours\b|\bwork(?:ing)?\b|how long|consult|консультац|hoe lang|combien de temps|how much|\bprice|\bcost|\bwhere\b|\baddress|\bparking` +
		`|\bna (?:de|het|mijn|een)\b|\bnadat\b|smeren|aanbreng|\bwass|douch|\bzwem|\bzon\b|zonnebank|\bbruin|crème|creme|\bolie|\bzweet|\bscheren|\bkrabben|\bnat\b|genez|roodheid|\brood\b|zwelling|\bjeuk|\bpijn|resulta|nazorg|verzorg|geopend|gesloten|openingsuren|openingstijden|\bprijs|\bkost|\bwaar\b|\badres` +
		`|\baprès(?:[^-]|$)|appliqu|\blaver|\blavez|douche|\bbain|\bnager|piscine|soleil|bronz|maquill|\bhuile|transpir|\brase|\bgratt|mouill|guéri|rougeur|gonfl|démang|douleur|résultat|\bsoins?\b|ouvert|fermé|horaires|\bprix|combien|\bcoût|\boù\b|adresse` +
		`|после|нанос|мазать|мыть|умыва|душ|ванн|плава|бассейн|солн|загар|загора|макияж|крем|масл|потет|брить|чеса|мочить|заживл|покраснен|от[её]к|зуд|раздраж|болит|результат|уход|работаете|открыт|закрыт|часы работы|цена|стоит|сколько|где|адрес` +
		`|після|мити|вмива|басейн|сонц|засмаг|макіяж|олі[яю]|пітні|голит|чуха|мочит|загоє|почервон|набряк|свербі|подразн|болить|догляд|працюєте|відчин|зачин|години роботи|ціна|коштує|скільки|\bде\b|адреса`)
	// whenAsked are the question words for when.
	whenAsked = regexp.MustCompile(`(?i)\bwhen\b|\bwanneer\b|\bquand\b|когда|коли`)
)

// asked reports whether s asks for free times, and whether it asks to
// come at a day or time; staff are the team's first names.
func asked(s string, staff []string) (times, come bool) {
	clocked := len(clockTimes(s)) > 0
	free := freeTimeWords.MatchString(s)
	at := clocked || when.MatchString(s) || whenAsked.MatchString(s)
	come = at && (free || comeWords.MatchString(s) || namesStaff(s, staff) || !otherTopics.MatchString(s))
	return come || free || clocked, come
}

// namesStaff reports whether s names one of staff, as a whole word.
func namesStaff(s string, staff []string) bool {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) })
	return slices.ContainsFunc(staff, func(n string) bool { return slices.Contains(words, strings.ToLower(n)) })
}

// asksWhen reports whether the visitor asks for free times, and whether
// they ask to come at a day or time. The message before the new one counts
// too, as they may name the treatment or the person only once asked ("With
// Zara?"); a message about something else ("How much is a pedicure?") is
// no answer to that, so the earlier ask to come then leaves only the days
// the reply names unchecked.
func asksWhen(history []Turn, message string, staff []string) (times, come bool) {
	times, come = asked(message, staff)
	for _, t := range slices.Backward(history) {
		if t.Role == RoleVisitor {
			before, cameBefore := asked(t.Text, staff)
			times = times || before
			come = come || cameBefore && !otherTopics.MatchString(message)
			break
		}
	}
	return times, come
}

// firstNames are the team's first names.
func firstNames(staff []content.Member) []string {
	var names []string
	for _, m := range staff {
		if f := strings.Fields(m.Name); len(f) > 0 {
			names = append(names, f[0])
		}
	}
	return names
}

// openingWords tell a clause about the studio's opening days and hours:
// the studio as its subject, never a person or an "open appointment".
var openingWords = regexp.MustCompile(`(?i)\b(?:we(?:'|’)re|we are|the studio is|the salon is) (?:open|closed)\b|\bopening (?:hours|days)\b|\bour hours\b` +
	`|\b(?:we zijn|wij zijn|de studio is|het salon is) (?:open|gesloten)\b|openingsuren|openingstijden` +
	`|\b(?:nous sommes|le studio est|le salon est) (?:ouverts?|fermés?)|horaires` +
	`|мы (?:открыты|закрыты|работаем|не работаем)|студия (?:открыта|закрыта|работает|не работает)|часы работы` +
	`|ми (?:відчинені|зачинені|працюємо|не працюємо)|студія (?:відчинена|зачинена|працює|не працює)|години роботи`)

var (
	// sentenceWithEnd is a sentence with the marks that end it.
	sentenceWithEnd = regexp.MustCompile(`[^.!?;\n]+[.!?;\n]*`)
	// clause splits a sentence at commas and the words that join clauses:
	// "We're open tomorrow and Zara can fit you in" is two claims.
	clause = regexp.MustCompile(`(?i),|\s+(?:and|but|so|then|plus|en|maar|dus|want|et|mais|donc|alors|и|но|а|і|й|та|але|тож)\s+`)
	// questionStart is a clause that starts by asking: "Which treatment,
	// and with whom?".
	questionStart = regexp.MustCompile(`(?i)^\s*(?:what|which|who|whom|whose|when|where|why|how|would|could|can|do|does|did|is|are|shall|should|will|may|with whom` +
		`|wat|welke?|wie|wanneer|waar|waarom|hoe|wilt?|zou|kan|kunt|heb|hebt|bent|met wie` +
		`|quel(?:le)?s?|qui|que|qu['’]|quand|où|pourquoi|comment|est-ce|voulez|souhaitez|préférez|avec qui` +
		`|что|как(?:ой|ую|ая|ое|ие)?|кто|когда|где|почему|хотите|с кем` +
		`|що|як(?:ий|у|а|е|і)?|хто|коли|де|чому|бажаєте|хочете|з ким)(?:[^\p{L}]|$)`)
)

// checked reports whether sentence s says nothing without a lookup: each
// clause is about the opening hours or, in a question, asks.
func checked(s string) bool {
	question := strings.Contains(s, "?")
	for _, c := range clause.Split(s, -1) {
		switch {
		case strings.IndexFunc(c, unicode.IsLetter) < 0:
		case openingWords.MatchString(c) || strings.Contains(c, hoursToken):
		case question && (strings.Contains(c, "?") || questionStart.MatchString(c)):
		default:
			return false
		}
	}
	return true
}

// namesWhen reports whether t, asked when the visitor can come, names a
// day or a part of the day in a sentence that says more than the opening
// hours or a question back: without a lookup, whatever it says about that
// day is unchecked.
func namesWhen(t string) bool {
	return slices.ContainsFunc(sentenceWithEnd.FindAllString(t, -1), func(s string) bool {
		return when.MatchString(s) && !checked(s)
	})
}

// asksBack reports whether t only asks the visitor something or tells them
// the studio's opening hours, clause by clause ("Yes, we're open" says
// yes): all the chat can say, without a lookup, to a visitor asking to come.
func asksBack(t string) bool {
	return !slices.ContainsFunc(sentenceWithEnd.FindAllString(t, -1), func(s string) bool { return !checked(s) })
}

// hoursToken is what the model writes for the published opening hours.
const hoursToken = "{opening_hours}"

// answer uses the recorded lookup outcomes as the entire availability reply.
// Without a lookup, text that says when an appointment is free, or anything
// but a question back to a visitor asking to come, is withheld, and the
// visitor told the chat confirms free times only from the calendar.
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
			case claimsAvailability(t) || r.asksWhen && namesWhen(t) || r.asksToCome && !asksBack(t):
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
