package views

import (
	"cmp"
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/lieranderl/aestheticlab/internal/content"
)

// priceCategory is one tab of the pricelist: a service group, or the
// merged laser category with its areas as lists.
type priceCategory struct {
	ID          string
	Title       string
	Description string
	// From is the category's "From € 40", or "".
	From  string
	Lists []priceList
	// Group is the category's service group, for the Book button.
	Group *content.ServiceGroup
}

// priceList is a list of treatments: the category's own, or one laser area.
type priceList struct {
	ID string
	// Label is the laser area's tab label ("Face"); "" for a category's
	// own list.
	Label string
	// Title names the list in headings and analytics ("Laser · Face").
	Title  string
	Main   []content.Service
	AddOns []content.Service
	// Cover is the photo the pricelist header shows for the list.
	Cover string
}

// pricelist builds the pricelist's categories from the content.
func pricelist(ctx context.Context, loc *content.Localized) []priceCategory {
	grouped := loc.Categories()
	laser := content.LaserSubcategories(grouped)
	var out []priceCategory
	for _, cat := range content.DisplayCategories(grouped) {
		title := upperFirst(categoryTitle(ctx, cat))
		pc := priceCategory{ID: cat.ID, Title: title, Description: categoryDescription(ctx, cat.Group), Group: cat.Group}
		if from, ok := content.StartingPrice(cat.Services); ok {
			pc.From = tr(ctx, "From") + " " + price(ctx, from)
		}
		lists := []content.Category{cat}
		if cat.ID == content.LaserID && len(laser) > 0 {
			lists = laser
		}
		for _, list := range lists {
			pl := priceList{ID: list.ID, Title: title, Cover: coverImage(categoryImageName(list))}
			if cat.ID == content.LaserID && len(laser) > 0 {
				pl.Label = laserLabel(ctx, list)
				pl.Title = title + " · " + pl.Label
			}
			for _, s := range list.Services {
				if content.IsAddOn(s) {
					pl.AddOns = append(pl.AddOns, s)
				} else {
					pl.Main = append(pl.Main, s)
				}
			}
			pc.Lists = append(pc.Lists, pl)
		}
		out = append(out, pc)
	}
	return out
}

// categoryTitle is a category's display name: its group's (translated)
// name, "Laser" for the merged laser category, else the default label.
func categoryTitle(ctx context.Context, cat content.Category) string {
	if cat.ID == content.LaserID {
		return tr(ctx, "Laser")
	}
	if cat.Group != nil && cat.Group.Name != "" {
		return cat.Group.Name
	}
	return tr(ctx, "All services listed alphabetically — select a category above to filter by treatment type.")
}

// categoryImageName is the name a category's images key on: the English
// group name, "laser" for the merged laser category.
func categoryImageName(cat content.Category) string {
	if cat.ID == content.LaserID {
		return "laser"
	}
	if cat.Group != nil {
		return cmp.Or(cat.Group.NameEN, cat.Group.Name)
	}
	return "universal"
}

// categoryDescription is a category's one-line description, chosen by its
// English name.
func categoryDescription(ctx context.Context, g *content.ServiceGroup) string {
	name := ""
	if g != nil {
		name = strings.ToLower(cmp.Or(g.NameEN, g.Name))
	}
	switch {
	case strings.Contains(name, "manicure"):
		return tr(ctx, "Clean, precise manicure care — from classic polish to structured gel — tailored to your nail health and style. Every finish is checked under magnification.")
	case strings.Contains(name, "pedicure"):
		return tr(ctx, "Thorough, hygienic pedicure treatments that focus on foot health, comfort, and a polished look. Performed with attention to detail and a gentle touch.")
	case strings.Contains(name, "brows"), strings.Contains(name, "lashes"):
		return tr(ctx, "From natural shaping and tinting to lamination that defines and lifts — expert brow and lash services that enhance your features with a clean, refined finish.")
	case strings.Contains(name, "laser"), strings.Contains(name, "removal"):
		return tr(ctx, "Effective diode laser hair removal performed by trained professionals in a calm, clinical setting. Treatment packages designed for real, lasting results.")
	case strings.Contains(name, "wax"):
		return tr(ctx, "Quick, precise waxing for brows, upper lip, chin, cheeks and the full face.")
	}
	return tr(ctx, "Every treatment is performed with care and precision. Choose a category to explore what we offer.")
}

var laserPrefix = regexp.MustCompile(`(?i)laser hair removal`)

// laserLabel is a laser area's tab label: "Laser hair removal FACE" → "Face".
func laserLabel(ctx context.Context, cat content.Category) string {
	name := categoryImageName(cat)
	switch strings.ToLower(strings.TrimSpace(laserPrefix.ReplaceAllString(name, ""))) {
	case "face":
		return tr(ctx, "Face")
	case "body":
		return tr(ctx, "Body")
	case "combo":
		return tr(ctx, "Sets")
	case "male":
		return tr(ctx, "Men")
	}
	if cat.Group != nil && cat.Group.Name != "" {
		return cat.Group.Name
	}
	return tr(ctx, "Laser")
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// imageKey is a category name as the image files are named: lower-case,
// hyphenated, with the names that differ mapped to their files.
func imageKey(name string) string {
	key := strings.Trim(nonAlnum.ReplaceAllString(strings.ToLower(name), "-"), "-")
	switch key {
	case "brows-lashes":
		return "brows"
	case "laser-hair-removal-face":
		return "laser-face"
	case "laser-hair-removal-body":
		return "laser-body"
	case "laser-hair-removal-combo":
		return "laser-combo"
	case "laser-hair-removal-male":
		return "laser-male"
	}
	return key
}

// coverImage is a category's cover photo (services/), else the universal
// gallery photo.
func coverImage(name string) string {
	switch key := imageKey(name); key {
	case "face-waxing", "manicure", "pedicure", "laser":
		return "services/" + key + ".jpg"
	case "brows", "laser-body", "laser-combo", "laser-face", "laser-male":
		return "services/" + key + ".webp"
	}
	return "gallery/universal.jpg"
}

// selectedCategory is the category the URL's ?category= names, else the
// first.
func selectedCategory(ctx context.Context, cats []priceCategory) string {
	want := queryParam(ctx, "category")
	if slices.ContainsFunc(cats, func(c priceCategory) bool { return c.ID == want }) {
		return want
	}
	if len(cats) > 0 {
		return cats[0].ID
	}
	return ""
}

// pricelistSignals are the pricelist's signals: the selected category, the
// selected laser area and the expanded treatment.
func pricelistSignals(category string, cats []priceCategory) string {
	laser := ""
	for _, c := range cats {
		if c.ID == content.LaserID && len(c.Lists) > 0 {
			laser = c.Lists[0].ID
		}
	}
	return "{category: " + jsString(category) + ", laser: " + jsString(laser) + ", open: ''}"
}

// jsMap is a JavaScript object literal from keys to texts, for expressions
// that pick a label by the selected laser area.
func jsMap(lists []priceList, value func(priceList) string) string {
	m := map[string]string{}
	for _, l := range lists {
		m[l.ID] = value(l)
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// listHeading is the expression for a laser category's heading: the
// category and the selected area.
func listHeading(c priceCategory) string {
	return jsMap(c.Lists, func(l priceList) string { return l.Title }) + "[$laser]"
}

// listCount is the expression for the selected list's "N Treatments".
func listCount(ctx context.Context, c priceCategory) string {
	return jsMap(c.Lists, func(l priceList) string { return countLabel(ctx, l) }) + "[$laser]"
}

func countLabel(ctx context.Context, l priceList) string {
	return strconv.Itoa(len(l.Main)) + " " + tr(ctx, "Treatments")
}

// cover is a photo the pricelist header can show, with the expression that
// makes it the visible one.
type cover struct {
	Image  string
	Active string // Datastar expression
	// Initially is whether it shows before any selection changes.
	Initially bool
}

// covers are the header photos: one per category, laser areas instead of
// the laser category, cross-faded by the selection.
func covers(cats []priceCategory, selected string) []cover {
	var out []cover
	for _, c := range cats {
		if c.Lists[0].Label != "" { // the laser category, shown by area
			for i, l := range c.Lists {
				out = append(out, cover{
					Image:     l.Cover,
					Active:    "$category === 'laser' && $laser === " + jsString(l.ID),
					Initially: selected == content.LaserID && i == 0,
				})
			}
			continue
		}
		out = append(out, cover{Image: c.Lists[0].Cover, Active: "$category === " + jsString(c.ID), Initially: selected == c.ID})
	}
	return out
}

// categoryViewed records service_category_viewed for a tab.
func categoryViewed(c priceCategory) string {
	return track("service_category_viewed", map[string]string{"category_id": c.ID, "service_category": c.Title, "placement": "pricelist_tabs"})
}

// pricingViewed records pricing_viewed when the pricelist opens.
func pricingViewed(loc *content.Localized) string {
	return "aesthetic.track('pricing_viewed', {placement: 'pricelist_page', category_count: " + strconv.Itoa(len(loc.ServiceGroups)) +
		", service_count: " + strconv.Itoa(len(loc.Services)) + "})"
}

// selectCategory is a tab's click: select it, collapse the open treatment,
// put it in the URL and record it.
func selectCategory(c priceCategory) string {
	return "$category = " + jsString(c.ID) + "; $open = ''; history.replaceState(null, '', '?category=' + encodeURIComponent(" +
		jsString(c.ID) + ") + location.hash); " + categoryViewed(c)
}

// is is the expression "signal === id".
func is(signal, id string) string { return "$" + signal + " === " + jsString(id) }
