package views

import (
	"cmp"
	"context"
	"net/url"
	"slices"
	"strconv"

	"github.com/lieranderl/aestheticlab/internal/content"
)

// treatments is the home page's services section in one state: the
// category overview, a category's treatments, or the laser category's areas.
type treatments struct {
	loc *content.Localized
	// Display are the categories with the laser groups merged; Laser are the
	// laser areas.
	Display, Laser []content.Category
	// Category and Area are the selection; "" for none.
	Category, Area string
}

// selectTreatments is the section for the URL's ?treatment= and
// ?treatmentArea=, falling back to the overview for unknown ones.
func selectTreatments(loc *content.Localized, query url.Values) treatments {
	grouped := loc.Categories()
	t := treatments{loc: loc, Display: content.DisplayCategories(grouped), Laser: content.LaserSubcategories(grouped)}
	t.Category, t.Area = content.Selection(t.Display, t.Laser, query.Get("treatment"), query.Get("treatmentArea"))
	return t
}

func find(cats []content.Category, id string) (content.Category, bool) {
	i := slices.IndexFunc(cats, func(c content.Category) bool { return c.ID == id })
	if i < 0 {
		return content.Category{}, false
	}
	return cats[i], true
}

// detail is the category whose treatments show: the selected laser area,
// else the selected category.
func (t treatments) detail() (content.Category, bool) {
	if t.Area != "" {
		return find(t.Laser, t.Area)
	}
	return find(t.Display, t.Category)
}

// choosingArea is the laser category selected with no area: its areas show
// as tiles.
func (t treatments) choosingArea() bool {
	return t.Category == content.LaserID && t.Area == "" && len(t.Laser) > 0
}

// tileTitle is a tile's or chip's name: the group's name, "Laser" for the
// merged laser category.
func tileTitle(ctx context.Context, c content.Category) string {
	return upperFirst(categoryTitle(ctx, c))
}

// detailTitle is the treatments heading, as getDisplayCategoryName names
// it: the category's group (the merged laser category has the first laser
// group's), else its title.
func detailTitle(ctx context.Context, c content.Category) string {
	if c.Group != nil && c.Group.Name != "" {
		return upperFirst(c.Group.Name)
	}
	return tileTitle(ctx, c)
}

// fromPrice is "From €40", or "".
func fromPrice(ctx context.Context, services []content.Service) string {
	if p, ok := content.StartingPrice(services); ok {
		return tr(ctx, "From") + " " + price(ctx, p)
	}
	return ""
}

// treatmentCount is "8 Treatments", add-ons not counted.
func treatmentCount(ctx context.Context, services []content.Service) string {
	return strconv.Itoa(len(content.MainServices(services))) + " " + tr(ctx, "Treatments")
}

// detailSummary is "From €40 · 8 Treatments".
func detailSummary(ctx context.Context, c content.Category) string {
	if from := fromPrice(ctx, c.Services); from != "" {
		return from + " · " + treatmentCount(ctx, c.Services)
	}
	return treatmentCount(ctx, c.Services)
}

// ordered is a category's treatments, add-ons last.
func ordered(services []content.Service) []content.Service {
	var main, addOns []content.Service
	for _, s := range services {
		if content.IsAddOn(s) {
			addOns = append(addOns, s)
		} else {
			main = append(main, s)
		}
	}
	return append(main, addOns...)
}

// serviceCategory names a treatment's group for analytics.
func (t treatments) serviceCategory(ctx context.Context, s content.Service, fallback content.Category) string {
	if g := slices.IndexFunc(t.loc.ServiceGroups, func(g content.ServiceGroup) bool { return g.ID == s.GroupID }); g >= 0 {
		return t.loc.ServiceGroups[g].Name
	}
	return categoryTitle(ctx, fallback)
}

// openTreatment is the expression that shows category (and laser area):
// the URL first, so the server renders the same state on reload and the
// back button restores the previous one, then the section from the server.
func openTreatment(ctx context.Context, category, area string, c content.Category, placement string) string {
	query := "?treatment=" + url.QueryEscape(category)
	if area != "" {
		query += "&treatmentArea=" + url.QueryEscape(area)
	}
	id := cmp.Or(area, category)
	return "history.pushState(null, '', " + jsString(query+"#services") + "); " + sectionGet(jsString(query)) + "; " +
		track("service_category_viewed", map[string]string{
			"category_id": id, "service_category": tileTitle(ctx, c), "placement": placement,
			"service_count": strconv.Itoa(len(c.Services)),
		})
}

// sectionGet fetches the services section for url (a JavaScript
// expression), cancelling any section request still under way, whatever its
// URL.
func sectionGet(url string) string {
	return "aesthetic.loadSection(() => @get(" + url + ", {requestCancellation: aesthetic.sectionSignal()}))"
}

// The back buttons' and the browser back button's expressions.
var (
	backToOverview    = "history.pushState(null, '', location.pathname + '#services'); " + sectionGet("location.pathname")
	backToLaser       = "history.pushState(null, '', '?treatment=laser#services'); " + sectionGet("'?treatment=laser'")
	sectionOnPopState = sectionGet("location.pathname + location.search")
)

// lowestAddOn is the lowest add-on price, for the services note.
func (t treatments) lowestAddOn(ctx context.Context) string {
	if p, ok := content.LowestAddOnPrice(t.loc.Services); ok {
		return price(ctx, p)
	}
	return ""
}

// tileOpen is a tile's or chip's expression: a category from the overview,
// an area from the laser tiles.
func tileOpen(ctx context.Context, t treatments, c content.Category, columns string) string {
	if columns == "laser" {
		return openTreatment(ctx, content.LaserID, c.ID, c, "laser_subgroup")
	}
	return openTreatment(ctx, c.ID, "", c, "services_overview")
}

// revealTreatments brings a freshly patched section into view: the
// treatments (focusing their heading for screen readers) or, for the
// overview, the section top. The expression names the selection: a patch
// morphs #services-body in place, and Datastar runs a data-init again only
// when its value changes.
func revealTreatments(t treatments) string {
	selection := "void " + jsString("services:"+t.Category+"/"+t.Area) + "; "
	if t.Category == "" {
		return selection + "aesthetic.revealTreatments('services')"
	}
	return selection + "aesthetic.revealTreatments('service-details-card'); " +
		"document.getElementById('service-details-heading')?.focus({preventScroll: true})"
}

// readMore toggles a clamped description and its button.
func readMore(ctx context.Context) string {
	return "el.previousElementSibling.classList.toggle('line-clamp-3'); " +
		"el.setAttribute('aria-expanded', el.previousElementSibling.classList.contains('line-clamp-3') ? 'false' : 'true'); " +
		"el.textContent = el.getAttribute('aria-expanded') === 'true' ? " + jsString(tr(ctx, "Read less")) + " : " + jsString(tr(ctx, "Read more"))
}

// minutesNoBreak is "60 min" with a no-break space, as the service rows
// write it.
func minutesNoBreak(ctx context.Context, d float64) string {
	return strconv.FormatFloat(d, 'f', -1, 64) + " " + tr(ctx, "min")
}

// pricingClicked records a link to the price list.
func pricingClicked(placement string) string {
	return track("pricing_link_clicked", map[string]string{"placement": placement})
}

// consultationClicked records the consultation link, an Instagram message.
func consultationClicked() string {
	return track("instagram_clicked", map[string]string{"placement": "services_consultation", "target_type": "message", "link_url": instagramMessage})
}
