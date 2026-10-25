package content_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/content"
	"github.com/lieranderl/aestheticlab/internal/i18n"
)

// contact is a complete contact document.
func contact() content.Document {
	return content.Document{
		"email":      "hello@example.com",
		"open_hours": map[string]any{"start_week_day": "Monday", "end_week_day": "Saturday", "from": "10:00", "to": "18:00"},
		"location":   map[string]any{"name": "Leuven", "address": "Main Street 1", "link": "https://maps.example.com/location"},
		"parking": []any{
			map[string]any{"name": "Safe", "link": "https://maps.example.com/parking"},
			map[string]any{"name": "Unsafe", "link": "javascript:alert(1)"},
		},
	}
}

// docs is complete content: one group, one service, one member of staff.
func docs() content.Documents {
	return content.Documents{
		ServiceGroups: []content.Document{{
			"id": int64(1), "active": true, "name": "Manicure", "name_fr": "Manucure", "name_nl": "", "name_ru": "Маникюр", "priority": int64(10),
			"internal_note": "never shown",
		}},
		Services: []content.Document{{
			"id": int64(11), "group_id": int64(1), "active": true, "name": "Classic manicure", "name_fr": "Manucure classique",
			"description": "English description", "description_fr": "Description française", "description_nl": "",
			"duration": int64(60), "price": int64(50), "priority": int64(10),
		}},
		Staff: []content.Document{{
			"id": int64(2), "active": true, "name": "Julia", "photo_url": "julia.jpg", "about": "English bio", "about_fr": "Bio française", "role": "Nail artist",
		}},
		Contact: contact(),
	}
}

func parse(t *testing.T, d content.Documents) *content.Content {
	t.Helper()
	c, err := content.Parse(d)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return c
}

func TestContentIsLocalizedWithEnglishAsTheFallback(t *testing.T) {
	c := parse(t, docs())
	fr, nl := c.In(i18n.FrBE), c.In(i18n.NlBE)

	if want := []content.ServiceGroup{{ID: "1", Name: "Manucure", NameEN: "Manicure", Priority: 10}}; !reflect.DeepEqual(fr.ServiceGroups, want) {
		t.Errorf("fr groups = %+v, want %+v", fr.ServiceGroups, want)
	}
	if want := []content.Service{{
		ID: "11", GroupID: "1", Name: "Manucure classique", NameEN: "Classic manicure",
		Description: "Description française", Duration: 60, Price: 50,
	}}; !reflect.DeepEqual(fr.Services, want) {
		t.Errorf("fr services = %+v, want %+v", fr.Services, want)
	}
	if want := []content.Member{{ID: 2, Name: "Julia", Photo: "julia.jpg", About: "Bio française", Role: "Nail artist"}}; !reflect.DeepEqual(fr.Staff, want) {
		t.Errorf("fr staff = %+v, want %+v", fr.Staff, want)
	}
	// Empty translations fall back to English.
	if got := nl.ServiceGroups[0].Name; got != "Manicure" {
		t.Errorf("nl group name = %q, want the English name", got)
	}
	if got := nl.Services[0].Description; got != "English description" {
		t.Errorf("nl description = %q, want the English one", got)
	}
	if got := c.In(i18n.UkBE).Staff[0].About; got != "English bio" {
		t.Errorf("uk about = %q, want the English one", got)
	}
	if got := c.In(i18n.RuBE).ServiceGroups[0].Name; got != "Маникюр" {
		t.Errorf("ru group name = %q", got)
	}
	if got := c.In("de-DE"); got != c.In(i18n.EnBE) {
		t.Errorf("In(de-DE) is not the English content")
	}
}

func TestAnEnglishGroupNameOverridesTheBaseName(t *testing.T) {
	d := docs()
	d.ServiceGroups[0]["name_en"] = "Manicure & care"
	c := parse(t, d)
	if got := c.In(i18n.EnBE).ServiceGroups[0]; got.Name != "Manicure & care" || got.NameEN != "Manicure & care" {
		t.Errorf("en group = %+v, want name_en as both names", got)
	}
	if got := c.In(i18n.NlBE).ServiceGroups[0].Name; got != "Manicure & care" {
		t.Errorf("nl group name = %q, want the English name", got)
	}
}

func TestInactiveAndMalformedDocumentsAreLeftOut(t *testing.T) {
	d := docs()
	d.ServiceGroups = append(d.ServiceGroups,
		content.Document{"id": "missing-fields", "active": true},
		content.Document{"id": int64(3), "name": "Hidden", "priority": int64(1), "active": false},
		content.Document{"id": int64(4), "name": "No active field", "priority": int64(1)},
	)
	d.Services = append(d.Services,
		content.Document{"id": "missing-fields", "active": true},
		content.Document{"id": int64(12), "group_id": int64(1), "name": "Price as text", "duration": int64(1), "price": "50", "active": true},
	)
	d.Staff = append(d.Staff,
		content.Document{"id": "invalid", "name": nil, "active": true},
		content.Document{"id": 2.5, "name": "Fraction", "active": true},
	)
	en := parse(t, d).In(i18n.EnBE)
	if len(en.ServiceGroups) != 1 || len(en.Services) != 1 || len(en.Staff) != 1 {
		t.Errorf("parsed %d groups, %d services, %d staff; want 1 of each", len(en.ServiceGroups), len(en.Services), len(en.Staff))
	}
}

func TestGroupsAndServicesFollowPriorityAndStaffFollowID(t *testing.T) {
	d := docs()
	d.ServiceGroups = append(d.ServiceGroups,
		content.Document{"id": int64(20), "active": true, "name": "Pedicure", "priority": int64(5)},
		content.Document{"id": int64(3), "active": true, "name": "Brows", "priority": int64(10)},
	)
	d.Services = append(d.Services,
		content.Document{"id": int64(9), "group_id": int64(1), "active": true, "name": "First", "duration": int64(30), "price": int64(20), "priority": int64(1)},
	)
	d.Staff = append(d.Staff,
		content.Document{"id": int64(10), "active": true, "name": "Zara"},
		content.Document{"id": int64(1), "active": true, "name": "Lera"},
	)
	en := parse(t, d).In(i18n.EnBE)
	var groups, services, staff []string
	for _, g := range en.ServiceGroups {
		groups = append(groups, g.ID)
	}
	for _, s := range en.Services {
		services = append(services, s.ID)
	}
	for _, m := range en.Staff {
		staff = append(staff, m.Name)
	}
	// Equal priorities keep numeric ID order (3 before 1 would be text order).
	if want := []string{"20", "1", "3"}; !reflect.DeepEqual(groups, want) {
		t.Errorf("groups = %v, want %v", groups, want)
	}
	if want := []string{"9", "11"}; !reflect.DeepEqual(services, want) {
		t.Errorf("services = %v, want %v", services, want)
	}
	if want := []string{"Lera", "Julia", "Zara"}; !reflect.DeepEqual(staff, want) {
		t.Errorf("staff = %v, want %v", staff, want)
	}
}

func TestContactKeepsOnlySafeLinks(t *testing.T) {
	got := parse(t, docs()).In(i18n.EnBE).Contact
	want := content.Contact{
		Email:    "hello@example.com",
		Hours:    content.OpenHours{StartWeekDay: "Monday", EndWeekDay: "Saturday", From: "10:00", To: "18:00"},
		Location: content.Place{Name: "Leuven", Address: "Main Street 1", Link: "https://maps.example.com/location"},
		Parking:  []content.Place{{Name: "Safe", Link: "https://maps.example.com/parking"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("contact = %+v, want %+v", got, want)
	}
}

func TestIncompleteContentIsRejected(t *testing.T) {
	unsafeLocation := docs()
	unsafeLocation.Contact["location"] = map[string]any{"name": "Leuven", "address": "Main Street 1", "link": "javascript:alert(1)"}
	noHours := docs()
	delete(noHours.Contact, "open_hours")
	noGroups := docs()
	noGroups.ServiceGroups = nil
	noServices := docs()
	noServices.Services[0]["active"] = false
	for name, d := range map[string]content.Documents{
		"unsafe location link": unsafeLocation, "no open hours": noHours,
		"no groups": noGroups, "no active services": noServices, "nothing": {},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := content.Parse(d); !errors.Is(err, content.ErrIncomplete) {
				t.Errorf("Parse() error = %v, want ErrIncomplete", err)
			}
		})
	}
}

func TestStaffPhotosAreBundledNamesOrWebLinks(t *testing.T) {
	for photo, want := range map[any]string{
		"julia":                              "julia",
		"julia.jpg":                          "julia.jpg",
		"https://cdn.example.com/julia.webp": "https://cdn.example.com/julia.webp",
		"javascript:alert(1)":                "",
		"../secrets/julia.jpg":               "",
		"data:image/png;base64,AAAA":         "",
		"//cdn.example.com/julia.webp":       "",
		int64(42):                            "",
	} {
		d := docs()
		d.Staff[0]["photo_url"] = photo
		if got := parse(t, d).In(i18n.EnBE).Staff[0].Photo; got != want {
			t.Errorf("photo_url %v = %q, want %q", photo, got, want)
		}
	}
}

func TestJSONNumbersParseLikeFirestoreIntegers(t *testing.T) {
	d := docs()
	d.ServiceGroups[0]["id"], d.ServiceGroups[0]["priority"] = 635110.0, 40.0
	d.Services[0]["group_id"], d.Services[0]["price"] = 635110.0, 37.5
	en := parse(t, d).In(i18n.EnBE)
	if got := en.ServiceGroups[0].ID; got != "635110" {
		t.Errorf("group ID = %q, want 635110", got)
	}
	if got := en.Services[0]; got.GroupID != "635110" || got.Price != 37.5 {
		t.Errorf("service = %+v, want group 635110 and price 37.5", got)
	}
}
