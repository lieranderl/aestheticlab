// Package content is the site's editable content (treatments, prices,
// staff and the studio's contact details), read from Firestore
// (docs/CONTENT.md), checked, and projected into each locale once per fetch.
package content

import (
	"cmp"
	"errors"
	"slices"
	"strconv"

	"github.com/lieranderl/aestheticlab/internal/i18n"
)

// Document is one stored document: its fields by name, as Firestore (int64
// numbers) or JSON (float64 numbers) decode them.
type Document = map[string]any

// Documents is the content as stored, and as the import file, the export
// and the embedded snapshot hold it: each collection's documents, and the
// one contact document (site/contact). Inactive documents are kept; Parse
// leaves them out.
type Documents struct {
	ServiceGroups []Document `json:"service_groups"`
	Services      []Document `json:"services"`
	Staff         []Document `json:"staff"`
	Contact       Document   `json:"contact"`
}

// ServiceGroup is a treatment category in one locale.
type ServiceGroup struct {
	ID string
	// Name is the locale's name, else the English one.
	Name string
	// NameEN is the English name, the same in every locale: images and
	// classification (laser, add-ons) key on it.
	NameEN   string
	Priority float64
}

// Service is a treatment in one locale.
type Service struct {
	ID      string
	GroupID string
	Name    string
	// NameEN is the English name, the same in every locale.
	NameEN      string
	Description string
	// Duration is in minutes.
	Duration float64
	// Price is in euros.
	Price float64
}

// Member is a member of staff in one locale.
type Member struct {
	ID   int64
	Name string
	// Photo is a bundled image name ("julia", "julia.jpg") or an http(s) URL.
	Photo string
	About string
	Role  string
}

// Contact is the studio's contact details. They have no translations.
type Contact struct {
	Email    string
	Hours    OpenHours
	Location Place
	Parking  []Place
}

// OpenHours is when the studio is open: StartWeekDay to EndWeekDay, From
// to To.
type OpenHours struct {
	StartWeekDay, EndWeekDay string
	From, To                 string
}

// Place is a named location with a maps link. Address is empty for parking.
type Place struct {
	Name    string
	Address string
	Link    string
}

// Localized is the content in one locale, in the order the site shows it:
// groups and services by priority, staff by ID.
type Localized struct {
	ServiceGroups []ServiceGroup
	Services      []Service
	Staff         []Member
	Contact       Contact
}

// Content is the checked content in every supported locale.
type Content struct {
	locales map[i18n.Locale]*Localized
}

// In is the content in l, or in English for a locale the site lacks.
func (c *Content) In(l i18n.Locale) *Localized {
	if loc, ok := c.locales[l]; ok {
		return loc
	}
	return c.locales[i18n.Default]
}

// ErrIncomplete is returned by Parse for content the site cannot show:
// no active service group or service, or no complete contact.
var ErrIncomplete = errors.New("content is incomplete")

// Parse checks docs and projects them into every locale. Malformed and
// inactive documents are left out, as the Qwik site's loaders leave them
// out; content left with no service groups, no services or no contact is
// ErrIncomplete, so a half-seeded database never replaces good content.
func Parse(docs Documents) (*Content, error) {
	groups := parseAll(docs.ServiceGroups, parseServiceGroup)
	services := parseAll(docs.Services, parseService)
	staff := parseAll(docs.Staff, parseMember)
	contact, ok := parseContact(docs.Contact)
	if len(groups) == 0 || len(services) == 0 || !ok {
		return nil, ErrIncomplete
	}
	// The Qwik site orders groups and services by priority (ascending) and
	// staff by ID; ties keep the documents' ID order, as here.
	slices.SortStableFunc(groups, func(a, b storedGroup) int { return cmp.Compare(a.priority, b.priority) })
	slices.SortStableFunc(services, func(a, b storedService) int { return cmp.Compare(a.priority, b.priority) })
	slices.SortStableFunc(staff, func(a, b storedMember) int { return cmp.Compare(a.id, b.id) })

	c := &Content{locales: map[i18n.Locale]*Localized{}}
	for _, l := range i18n.Supported() {
		loc := &Localized{Contact: contact}
		for _, g := range groups {
			loc.ServiceGroups = append(loc.ServiceGroups, g.in(l))
		}
		for _, s := range services {
			loc.Services = append(loc.Services, s.in(l))
		}
		for _, m := range staff {
			loc.Staff = append(loc.Staff, m.in(l))
		}
		c.locales[l] = loc
	}
	return c, nil
}

// parseAll parses the active documents parse accepts, sorted by ID.
func parseAll[T any](docs []Document, parse func(Document) (T, bool)) []T {
	sorted := slices.Clone(docs)
	slices.SortStableFunc(sorted, func(a, b Document) int { return compareIDs(readID(a, "id"), readID(b, "id")) })
	var out []T
	for _, doc := range sorted {
		if active, _ := doc["active"].(bool); !active {
			continue
		}
		if v, ok := parse(doc); ok {
			out = append(out, v)
		}
	}
	return out
}

// compareIDs orders numeric IDs as numbers and any others after them as
// text.
func compareIDs(a, b string) int {
	na, errA := strconv.ParseFloat(a, 64)
	nb, errB := strconv.ParseFloat(b, 64)
	switch {
	case errA == nil && errB == nil:
		return cmp.Compare(na, nb)
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	}
	return cmp.Compare(a, b)
}

// translated is a text and its translations by language ("ru", "nl", ...).
type translated struct {
	english string
	others  map[string]string
}

// readTranslated reads field and its field_<language> translations.
func readTranslated(doc Document, field, english string) translated {
	t := translated{english: english, others: map[string]string{}}
	for _, l := range i18n.Supported() {
		if lang := l.Language(); lang != "en" {
			t.others[lang] = readString(doc, field+"_"+lang)
		}
	}
	return t
}

// in is the text in l: its translation, else the English text.
func (t translated) in(l i18n.Locale) string {
	return cmp.Or(t.others[l.Language()], t.english)
}

type storedGroup struct {
	id       string
	name     translated
	priority float64
}

func parseServiceGroup(doc Document) (storedGroup, bool) {
	id, name := readID(doc, "id"), readString(doc, "name")
	priority, ok := readNumber(doc, "priority")
	if id == "" || name == "" || !ok {
		return storedGroup{}, false
	}
	return storedGroup{id: id, name: readTranslated(doc, "name", cmp.Or(readString(doc, "name_en"), name)), priority: priority}, true
}

func (g storedGroup) in(l i18n.Locale) ServiceGroup {
	return ServiceGroup{ID: g.id, Name: g.name.in(l), NameEN: g.name.english, Priority: g.priority}
}

type storedService struct {
	id, groupID       string
	name, description translated
	duration, price   float64
	priority          float64
}

func parseService(doc Document) (storedService, bool) {
	id, groupID, name := readID(doc, "id"), readID(doc, "group_id"), readString(doc, "name")
	duration, okDuration := readNumber(doc, "duration")
	price, okPrice := readNumber(doc, "price")
	if id == "" || groupID == "" || name == "" || !okDuration || !okPrice {
		return storedService{}, false
	}
	priority, _ := readNumber(doc, "priority")
	return storedService{
		id: id, groupID: groupID,
		name:        readTranslated(doc, "name", name),
		description: readTranslated(doc, "description", readString(doc, "description")),
		duration:    duration, price: price, priority: priority,
	}, true
}

func (s storedService) in(l i18n.Locale) Service {
	return Service{
		ID: s.id, GroupID: s.groupID, Name: s.name.in(l), NameEN: s.name.english,
		Description: s.description.in(l), Duration: s.duration, Price: s.price,
	}
}

type storedMember struct {
	id                int64
	name, photo, role string
	about             translated
}

func parseMember(doc Document) (storedMember, bool) {
	id, okID := readNumber(doc, "id")
	name := readString(doc, "name")
	if !okID || id != float64(int64(id)) || name == "" {
		return storedMember{}, false
	}
	return storedMember{
		id: int64(id), name: name, photo: readPhoto(doc, "photo_url"), role: readString(doc, "role"),
		about: readTranslated(doc, "about", readString(doc, "about")),
	}, true
}

func (m storedMember) in(l i18n.Locale) Member {
	return Member{ID: m.id, Name: m.name, Photo: m.photo, About: m.about.in(l), Role: m.role}
}

func parseContact(doc Document) (Contact, bool) {
	hours, okHours := doc["open_hours"].(map[string]any)
	location, okLocation := doc["location"].(map[string]any)
	if !okHours || !okLocation {
		return Contact{}, false
	}
	c := Contact{
		Email: readString(doc, "email"),
		Hours: OpenHours{
			StartWeekDay: readString(hours, "start_week_day"), EndWeekDay: readString(hours, "end_week_day"),
			From: readString(hours, "from"), To: readString(hours, "to"),
		},
		Location: Place{Name: readString(location, "name"), Address: readString(location, "address"), Link: readLink(location, "link")},
	}
	if c.Email == "" || c.Location.Name == "" || c.Location.Address == "" || c.Location.Link == "" || c.Hours.From == "" || c.Hours.To == "" {
		return Contact{}, false
	}
	parking, _ := doc["parking"].([]any)
	for _, p := range parking {
		row, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if name, link := readString(row, "name"), readLink(row, "link"); name != "" && link != "" {
			c.Parking = append(c.Parking, Place{Name: name, Link: link})
		}
	}
	return c, true
}
