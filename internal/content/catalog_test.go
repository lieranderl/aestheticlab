package content_test

import (
	"reflect"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/content"
)

func ids(cats []content.Category) []string {
	var out []string
	for _, c := range cats {
		out = append(out, c.ID)
	}
	return out
}

func serviceIDs(services []content.Service) []string {
	var out []string
	for _, s := range services {
		out = append(out, s.ID)
	}
	return out
}

func catalog() *content.Localized {
	return &content.Localized{
		ServiceGroups: []content.ServiceGroup{
			{ID: "manicure", Name: "Manicure", NameEN: "Manicure", Priority: 40},
			{ID: "laser-face", Name: "Laser face", NameEN: "Laser hair removal FACE", Priority: 20},
			{ID: "laser-body", Name: "Laser body", NameEN: "Laser hair removal BODY", Priority: 10},
			{ID: "empty", Name: "No services", NameEN: "No services", Priority: 99},
		},
		Services: []content.Service{
			{ID: "m70", GroupID: "manicure", Price: 70},
			{ID: "m40", GroupID: "manicure", Price: 40},
			{ID: "body", GroupID: "laser-body", Price: 30},
			{ID: "face", GroupID: "laser-face", Price: 25},
			{ID: "orphan", GroupID: "missing", Price: 65},
		},
	}
}

func TestServicesAreGroupedByPriorityAndPrice(t *testing.T) {
	cats := catalog().Categories()
	if want := []string{"manicure", "laser-face", "laser-body", "missing"}; !reflect.DeepEqual(ids(cats), want) {
		t.Fatalf("categories = %v, want %v", ids(cats), want)
	}
	if want := []string{"m40", "m70"}; !reflect.DeepEqual(serviceIDs(cats[0].Services), want) {
		t.Errorf("manicure services = %v, want cheapest first %v", serviceIDs(cats[0].Services), want)
	}
	if orphan := cats[3]; orphan.Group != nil || orphan.Priority != 0 {
		t.Errorf("orphan category = %+v, want no group and priority 0", orphan)
	}
}

func TestEqualPrioritiesKeepNumericGroupOrder(t *testing.T) {
	l := &content.Localized{
		ServiceGroups: []content.ServiceGroup{{ID: "10", Priority: 1}, {ID: "9", Priority: 1}},
		Services:      []content.Service{{ID: "a", GroupID: "10"}, {ID: "b", GroupID: "9"}},
	}
	if got, want := ids(l.Categories()), []string{"9", "10"}; !reflect.DeepEqual(got, want) {
		t.Errorf("categories = %v, want %v", got, want)
	}
}

func TestLaserGroupsMergeIntoOneCategory(t *testing.T) {
	cats := catalog().Categories()
	display := content.DisplayCategories(cats)
	if want := []string{"manicure", content.LaserID, "missing"}; !reflect.DeepEqual(ids(display), want) {
		t.Fatalf("display categories = %v, want %v", ids(display), want)
	}
	laser := display[1]
	if want := []string{"face", "body"}; !reflect.DeepEqual(serviceIDs(laser.Services), want) {
		t.Errorf("laser services = %v, want %v", serviceIDs(laser.Services), want)
	}
	if laser.Priority != 20 || laser.Group == nil || laser.Group.ID != "laser-face" {
		t.Errorf("laser category = %+v, want priority 20 and the face group", laser)
	}
	if got, want := ids(content.LaserSubcategories(cats)), []string{"laser-face", "laser-body"}; !reflect.DeepEqual(got, want) {
		t.Errorf("laser subcategories = %v, want %v", got, want)
	}
	noLaser := cats[:1]
	if got := content.DisplayCategories(noLaser); !reflect.DeepEqual(got, noLaser) {
		t.Errorf("DisplayCategories without laser = %v, want them unchanged", ids(got))
	}
}

func TestLaserClassificationUsesTheEnglishName(t *testing.T) {
	for name, want := range map[string]bool{
		"Laser Hair Removal Face": true, "Tattoo Removal": true, "laser hair removal": true, "Manicure": false,
	} {
		if got := content.IsLaser(&content.ServiceGroup{Name: "Vertaald", NameEN: name}); got != want {
			t.Errorf("IsLaser(%q) = %v, want %v", name, got, want)
		}
	}
	if content.IsLaser(nil) {
		t.Error("IsLaser(nil) = true")
	}
}

func TestOnlyKnownSelectionsAreKept(t *testing.T) {
	cats := catalog().Categories()
	display, laser := content.DisplayCategories(cats), content.LaserSubcategories(cats)
	for _, tc := range []struct{ category, subgroup, wantCategory, wantSubgroup string }{
		{"unknown", "unknown", "", ""},
		{"", "laser-face", "", ""},
		{content.LaserID, "laser-face", content.LaserID, "laser-face"},
		{content.LaserID, "unknown", content.LaserID, ""},
		{"manicure", "laser-face", "manicure", ""},
	} {
		c, s := content.Selection(display, laser, tc.category, tc.subgroup)
		if c != tc.wantCategory || s != tc.wantSubgroup {
			t.Errorf("Selection(%q, %q) = %q, %q; want %q, %q", tc.category, tc.subgroup, c, s, tc.wantCategory, tc.wantSubgroup)
		}
	}
}

func TestAddOnsNeverSetTheStartingPrice(t *testing.T) {
	repair := content.Service{ID: "repair", Name: "Extra nail repair (up to 3 nails)", Price: 5}
	removal := content.Service{ID: "removal", Name: "Coating removal (our salon)", Price: 10}
	express := content.Service{ID: "express", Name: "Express manicure + polish", Price: 35}
	extensions := content.Service{ID: "extensions", Name: "Extensions using dual forms", Price: 80}

	for _, tc := range []struct {
		s    content.Service
		want bool
	}{
		{repair, true}, {removal, true}, {content.Service{Name: "Extra nail design: french"}, true},
		{express, false}, {extensions, false},
		// Translated pages classify by the English name.
		{content.Service{Name: "Gellak verwijderen", NameEN: "Gel polish coating removal"}, true},
		{content.Service{Name: "Extra herstel", NameEN: "Manicure express"}, false},
	} {
		if got := content.IsAddOn(tc.s); got != tc.want {
			t.Errorf("IsAddOn(%q / %q) = %v, want %v", tc.s.Name, tc.s.NameEN, got, tc.want)
		}
	}
	if got, ok := content.StartingPrice([]content.Service{repair, removal, express, extensions}); !ok || got != 35 {
		t.Errorf("StartingPrice = %v, %v; want 35", got, ok)
	}
	if got, ok := content.StartingPrice([]content.Service{repair, removal}); !ok || got != 5 {
		t.Errorf("StartingPrice of add-ons only = %v, %v; want 5 (every service counts)", got, ok)
	}
	if _, ok := content.StartingPrice(nil); ok {
		t.Error("StartingPrice(nil) is set")
	}
	if got, ok := content.LowestAddOnPrice([]content.Service{express, removal, repair}); !ok || got != 5 {
		t.Errorf("LowestAddOnPrice = %v, %v; want 5", got, ok)
	}
	if _, ok := content.LowestAddOnPrice([]content.Service{express}); ok {
		t.Error("LowestAddOnPrice without add-ons is set")
	}
	if got := serviceIDs(content.MainServices([]content.Service{repair, removal})); !reflect.DeepEqual(got, []string{"repair", "removal"}) {
		t.Errorf("MainServices of add-ons only = %v, want both", got)
	}
}
