package content

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
)

// Category is a service group with its services, as the home page's
// service grid and the pricelist show them (src/shared/service-utils.ts).
type Category struct {
	// ID is the group's ID, or LaserID for the merged laser category.
	ID string
	// Group is the service group, nil for services whose group is missing.
	// The merged laser category has the first laser group's.
	Group *ServiceGroup
	// Services are by price, cheapest first.
	Services []Service
	// Priority orders categories, highest first.
	Priority float64
}

// LaserID is the ID of the category DisplayCategories merges every laser
// group into.
const LaserID = "laser"

// Categories groups the services by service group, highest priority first,
// each group's services cheapest first. Groups without services are left
// out; services of a missing group form a category with no Group.
func (l *Localized) Categories() []Category {
	var cats []Category
	index := map[string]int{}
	for _, s := range l.Services {
		i, ok := index[s.GroupID]
		if !ok {
			i = len(cats)
			index[s.GroupID] = i
			cat := Category{ID: s.GroupID}
			if g := slices.IndexFunc(l.ServiceGroups, func(g ServiceGroup) bool { return g.ID == s.GroupID }); g >= 0 {
				cat.Group, cat.Priority = &l.ServiceGroups[g], l.ServiceGroups[g].Priority
			}
			cats = append(cats, cat)
		}
		cats[i].Services = append(cats[i].Services, s)
	}
	for i := range cats {
		slices.SortStableFunc(cats[i].Services, byPrice)
	}
	// Equal priorities keep group ID order, as the Qwik site's object keys
	// (numeric IDs, ascending) kept it.
	slices.SortStableFunc(cats, func(a, b Category) int { return compareIDs(a.ID, b.ID) })
	slices.SortStableFunc(cats, byPriority)
	return cats
}

// IsLaser reports whether g is a laser (or other removal) group, which the
// site shows as subgroups of one laser category.
func IsLaser(g *ServiceGroup) bool {
	if g == nil {
		return false
	}
	name := strings.ToLower(cmp.Or(g.NameEN, g.Name))
	return strings.Contains(name, "laser") || strings.Contains(name, "removal")
}

// DisplayCategories is cats with every laser category merged into one,
// LaserID, holding all their services (cheapest first) at the highest
// laser priority.
func DisplayCategories(cats []Category) []Category {
	var out, laser []Category
	for _, c := range cats {
		if IsLaser(c.Group) {
			laser = append(laser, c)
		} else {
			out = append(out, c)
		}
	}
	if len(laser) == 0 {
		return cats
	}
	merged := Category{ID: LaserID, Group: laser[0].Group, Priority: laser[0].Priority}
	for _, c := range laser {
		merged.Services = append(merged.Services, c.Services...)
		merged.Priority = max(merged.Priority, c.Priority)
	}
	slices.SortStableFunc(merged.Services, byPrice)
	out = append(out, merged)
	slices.SortStableFunc(out, byPriority)
	return out
}

// LaserSubcategories is the laser categories of cats, highest priority
// first: the subgroups inside the merged laser category.
func LaserSubcategories(cats []Category) []Category {
	var out []Category
	for _, c := range cats {
		if IsLaser(c.Group) {
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, byPriority)
	return out
}

// Selection is the category and laser subgroup a page's URL selects, each
// "" unless display (or, for the subgroup, laser) has it. A subgroup is
// selected only inside the laser category.
func Selection(display, laser []Category, categoryID, subgroupID string) (category, subgroup string) {
	has := func(cats []Category, id string) bool {
		return id != "" && slices.ContainsFunc(cats, func(c Category) bool { return c.ID == id })
	}
	if !has(display, categoryID) {
		return "", ""
	}
	if categoryID == LaserID && has(laser, subgroupID) {
		return categoryID, subgroupID
	}
	return categoryID, ""
}

// addOn matches add-ons (repairs, nail art extras, coating removal), which
// are booked on top of a treatment.
var addOn = regexp.MustCompile(`(?i)^extra\b|coating removal`)

// IsAddOn reports whether s is an add-on. It classifies by the English
// name: a translated one (Dutch "verwijderen") would let add-ons through.
func IsAddOn(s Service) bool {
	return addOn.MatchString(strings.TrimSpace(cmp.Or(s.NameEN, s.Name)))
}

// MainServices is services without add-ons, or all of them when every one
// is an add-on.
func MainServices(services []Service) []Service {
	main := slices.DeleteFunc(slices.Clone(services), IsAddOn)
	if len(main) == 0 {
		return services
	}
	return main
}

// StartingPrice is the lowest price of a main treatment in services: a
// category's "from" price. Add-ons never set it.
func StartingPrice(services []Service) (float64, bool) {
	return lowestPrice(MainServices(services))
}

// LowestAddOnPrice is the lowest add-on price in services, if any is an
// add-on.
func LowestAddOnPrice(services []Service) (float64, bool) {
	return lowestPrice(slices.DeleteFunc(slices.Clone(services), func(s Service) bool { return !IsAddOn(s) }))
}

func lowestPrice(services []Service) (float64, bool) {
	if len(services) == 0 {
		return 0, false
	}
	return slices.MinFunc(services, byPrice).Price, true
}

func byPrice(a, b Service) int { return cmp.Compare(a.Price, b.Price) }

func byPriority(a, b Category) int { return cmp.Compare(b.Priority, a.Priority) }
