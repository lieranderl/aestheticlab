package views

import (
	"cmp"
	"encoding/json"
	"net/url"
	"strings"
)

// Booking is a Book button: what Timely's widget opens on and what
// analytics records. Every button
// opens the page's one booking dialog.
type Booking struct {
	// ID names the button in analytics (booking_id).
	ID   string
	Text string
	// AriaLabel names the button when its text is ambiguous.
	AriaLabel string
	Class     string
	// Location is Timely's location ID; empty is the studio's.
	Location, Category, Product, Staff string
	// Placement, ServiceID, ServiceName and ServiceCategory are analytics
	// parameters.
	Placement, ServiceID, ServiceName, ServiceCategory string
}

// widgetURL is the Timely widget for b, its parameters in the Qwik site's
// order.
func (b Booking) widgetURL() string {
	q := []string{"location=" + url.QueryEscape(b.location())}
	for _, p := range [][2]string{{"category", b.Category}, {"product", b.Product}, {"staff", b.Staff}} {
		if p[1] != "" {
			q = append(q, p[0]+"="+url.QueryEscape(p[1]))
		}
	}
	return bookingURL + "?" + strings.Join(q, "&")
}

func (b Booking) location() string { return cmp.Or(strings.TrimSpace(b.Location), bookingLocationID) }

// event is b's analytics parameters as JSON; empty ones are left out.
func (b Booking) event() string {
	params := map[string]string{
		"booking_id": b.ID, "placement": cmp.Or(b.Placement, b.ID), "booking_location": b.location(),
		"service_id": b.ServiceID, "service_name": b.ServiceName,
		"service_category": cmp.Or(b.ServiceCategory, b.Category), "booking_product": b.Product, "staff_id": b.Staff,
	}
	for k, v := range params {
		if v == "" {
			delete(params, k)
		}
	}
	data, err := json.Marshal(params)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// open is the Datastar expression that opens the booking dialog on b's
// widget and records booking_opened.
func (b Booking) open() string {
	return "$bookingUrl = " + jsString(b.widgetURL()) + "; $bookingTitle = " + jsString(b.Text) + "; $bookingLoaded = false; $bookingEvent = " + jsString(b.event()) +
		"; document.getElementById('booking').showModal(); aesthetic.track('booking_opened', JSON.parse($bookingEvent))"
}

// track is the expression that records event name with params.
func track(name string, params map[string]string) string {
	data, err := json.Marshal(params)
	if err != nil {
		data = []byte("{}")
	}
	return "aesthetic.track(" + jsString(name) + ", JSON.parse(" + jsString(string(data)) + "))"
}

// fadeDelay is the transition delay class for a FadeUp, rounded to 20ms
// and capped at 400ms as the Qwik site does. Tailwind sees only literal
// class names, hence the list.
func fadeDelay(ms int) string {
	switch min(400, max(0, (ms+10)/20*20)) {
	case 80:
		return "delay-80"
	case 100:
		return "delay-100"
	case 160:
		return "delay-160"
	case 200:
		return "delay-200"
	case 400:
		return "delay-400"
	}
	return ""
}

// scrollTo is the expression that scrolls to the element id names (less
// the 64px header), or to the top for "", smoothly unless the visitor
// prefers reduced motion.
func scrollTo(id string) string {
	target := "0"
	if id != "" {
		target = "document.getElementById(" + jsString(id) + ").getBoundingClientRect().top + window.scrollY - 64"
	}
	return "window.scrollTo({top: Math.max(0, " + target + "), behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'})"
}

// showScrollTop is the expression for the scroll-to-top button's
// visibility: past 600px, or 480px past the element id names.
func showScrollTop(id string) string {
	threshold := "600"
	if id != "" {
		threshold = "document.getElementById(" + jsString(id) + ").getBoundingClientRect().top + window.scrollY + 480"
	}
	return "$toTop = window.scrollY > " + threshold
}
