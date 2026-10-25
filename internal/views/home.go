package views

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/lieranderl/aestheticlab/internal/assets"
	"github.com/lieranderl/aestheticlab/internal/content"
)

// sloganSize sizes the hero headline on phones from the slogan's length, so
// one line fits in every locale (the Qwik site sets --slogan-chars inline,
// which the CSP forbids). The classes are literal for Tailwind;
// TestEverySloganHasItsSize keeps a case for each locale's slogan.
func sloganSize(slogan string) string {
	switch utf8.RuneCountInString(slogan) {
	case 25:
		return "text-[clamp(1.25rem,calc((100vw-3rem)/(25*0.46)),2.75rem)]"
	case 28:
		return "text-[clamp(1.25rem,calc((100vw-3rem)/(28*0.46)),2.75rem)]"
	case 30:
		return "text-[clamp(1.25rem,calc((100vw-3rem)/(30*0.46)),2.75rem)]"
	case 35:
		return "text-[clamp(1.25rem,calc((100vw-3rem)/(35*0.46)),2.75rem)]"
	}
	return "text-[clamp(1.25rem,calc((100vw-3rem)/(35*0.46)),2.75rem)]"
}

// galleryItem is a photo in the work grid and its lightbox.
type galleryItem struct {
	Image, Alt, Class, ImageClass, Sizes string
}

const smallSizes = "(min-width: 1280px) 19rem, (min-width: 768px) 25vw, 50vw"

// galleryItems are the work grid's photos, in lightbox order: a magazine
// grid with one large and one wide tile.
func galleryItems(ctx context.Context) []galleryItem {
	wide := "(min-width: 1280px) 38rem, (min-width: 768px) 50vw, 100vw"
	return []galleryItem{
		{"gallery/pink-shimmer.jpg", tr(ctx, "Pink shimmer gel manicure resting on a mohair knit"), "col-span-2 h-60 sm:h-80 md:row-span-2", "object-[center_45%]", wide},
		{"gallery/atelier/chrome-manicure.jpg", tr(ctx, "Chrome manicure at Aesthetic Lab"), "h-35 sm:h-56", "object-[center_47%]", smallSizes},
		{"gallery/cherry-gloss.jpg", tr(ctx, "Glossy dark cherry manicure"), "h-35 sm:h-56", "object-[center_50%]", smallSizes},
		{"gallery/lash-lift-result.jpg", tr(ctx, "Lifted, curled lashes after a lash lift"), "h-35 sm:h-56", "object-[center_70%]", smallSizes},
		{"gallery/tortoise.jpg", tr(ctx, "Brown and tortoiseshell gel manicure"), "h-35 sm:h-56", "object-[center_60%]", smallSizes},
		{"gallery/white-shimmer.jpg", tr(ctx, "White shimmer gel manicure"), "h-35 sm:h-56", "object-[center_45%]", smallSizes},
		{"gallery/lash-lift-process.jpg", tr(ctx, "Lash lift in progress, lashes set on a silicone shield"), "h-35 sm:h-56", "object-[center_55%]", smallSizes},
		{"gallery/pedicure5.jpg", tr(ctx, "Pedicure preparation at Aesthetic Lab"), "col-span-2 h-35 sm:h-56", "object-[center_40%]", wide},
	}
}

// lightboxImages is the lightbox's photos as a JavaScript array of
// {src, srcset, alt}, indexed by $lightbox.
func lightboxImages(ctx context.Context) string {
	type image struct {
		Src    string `json:"src"`
		SrcSet string `json:"srcset"`
		Alt    string `json:"alt"`
	}
	var out []image
	for _, item := range galleryItems(ctx) {
		img, _ := assets.Lookup(item.Image)
		out = append(out, image{img.Src, img.SrcSet, item.Alt})
	}
	data, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// clip is a short studio video.
type clip struct {
	ID, Video, Poster, Tag, Label string
}

func clips(ctx context.Context) []clip {
	return []clip{
		{"work-clip-0", "video/chrome.mp4", "video/chrome-poster.jpg", tr(ctx, "Chrome"), tr(ctx, "Chrome powder buffed onto a lavender gel nail")},
		{"work-clip-1", "video/cateye.mp4", "video/cateye-poster.jpg", tr(ctx, "Cat eye"), tr(ctx, "Cat-eye gel polish shimmering under a magnet")},
		{"work-clip-2", "video/nude.mp4", "video/nude-poster.jpg", tr(ctx, "Soft pink"), tr(ctx, "A finished soft pink gel manicure")},
		{"work-clip-3", "video/shimmer.mp4", "video/shimmer-poster.jpg", tr(ctx, "Foam wash"), tr(ctx, "Pale pink shimmer manicure rinsed with soft foam")},
	}
}

// videoSignal is the signal holding a clip's state ("playing", "paused",
// "ended"): hero-video → heroVideoState.
func videoSignal(id string) string {
	parts := strings.Split(id, "-")
	for i := 1; i < len(parts); i++ {
		parts[i] = upperFirst(parts[i])
	}
	return strings.Join(parts, "") + "State"
}

// videoLabel is the expression for a clip control's label: what a press
// does, then the clip's name when a page has several.
func videoLabel(ctx context.Context, id, name string) string {
	labels, _ := json.Marshal(map[string]string{
		"playing": tr(ctx, "Pause video"), "paused": tr(ctx, "Play video"), "ended": tr(ctx, "Replay video"),
	})
	suffix := ""
	if name != "" {
		suffix = " + " + jsString(": "+name)
	}
	return string(labels) + "[$" + videoSignal(id) + "]" + suffix
}

// videoLabelText is the control's label before any state change.
func videoLabelText(ctx context.Context, name string) string {
	if name != "" {
		return tr(ctx, "Play video") + ": " + name
	}
	return tr(ctx, "Play video")
}

// ratingLabel is "★★★★★ 5.0 · 60 Google reviews".
func ratingLabel(ctx context.Context) string {
	return "★★★★★ " + googleRating + " · " + trf(ctx, "%v Google reviews", googleReviews)
}

// durationRange is the shortest and longest treatment, add-ons left out:
// a five-minute add-on is not a treatment length.
func durationRange(loc *content.Localized) (lo, hi int, ok bool) {
	var ds []float64
	for _, s := range loc.Services {
		if !content.IsAddOn(s) && s.Duration > 0 {
			ds = append(ds, s.Duration)
		}
	}
	if len(ds) == 0 {
		return 0, 0, false
	}
	return int(slices.Min(ds)), int(slices.Max(ds)), true
}

// faqItem is a question with its answer, which is translated HTML.
type faqItem struct {
	Question, Answer string
}

func faqItems(ctx context.Context, loc *content.Localized) []faqItem {
	duration := tr(ctx, "Every treatment’s time is on the <a href='./pricelist'>price list</a>.")
	if lo, hi, ok := durationRange(loc); ok {
		duration = trf(ctx, "From %[1]v to %[2]v minutes, depending on the treatment. Every treatment’s time is on the <a href='./pricelist'>price list</a>.", lo, hi)
	}
	return []faqItem{
		{tr(ctx, "How long do treatments take?"), duration},
		{tr(ctx, "Where is the studio?"), trf(ctx, "%v, in the centre of Leuven. Directions and nearby parking are under <a href='#contact'>Visit</a>.", street+", "+zip+" "+city)},
		{tr(ctx, "How should I prepare for laser?"), tr(ctx, "Shave the area 24 hours before and avoid sun, tanning and self-tanner for at least 2 weeks. Read the full <a href='./notice?tab=laser'>care notes</a> before your visit.")},
		{tr(ctx, "Can I cancel or reschedule?"), tr(ctx, "Yes, at least 24 hours ahead in the booking system, or message us on <a href='https://www.instagram.com/aestheticlabbe' target='_blank' rel='noopener noreferrer'>Instagram</a> or by <a href='mailto:aestheticlabbe@gmail.com'>email</a>. Late cancellations may be charged.")},
		{tr(ctx, "Can I get advice before booking?"), tr(ctx, "Yes. If you are unsure which treatment suits you, <a href='https://ig.me/m/aestheticlabbe' target='_blank' rel='noopener noreferrer'>ask us</a> for a short, free consultation.")},
	}
}

// teamPhoto is a member's bundled photo ("julia" → julia.jpg), if any.
func teamPhoto(photo string) (string, bool) {
	name := strings.TrimSuffix(strings.ToLower(photo), ".jpg") + ".jpg"
	if _, ok := assets.Lookup(name); ok && photo != "" {
		return name, true
	}
	return "", false
}

var junior = regexp.MustCompile(`(?i)junior`)

// teamRole is a member's role, with "intro prices" for juniors.
func teamRole(ctx context.Context, m content.Member) string {
	role := m.Role
	if role == "" {
		role = tr(ctx, "Technician")
	}
	if junior.MatchString(m.Role) {
		role += " · " + tr(ctx, "intro prices")
	}
	return role
}

// initial is a name's first letter, shown when a member has no photo.
func initial(name string) string {
	r, _ := utf8.DecodeRuneInString(name)
	return string(r)
}

// addressLines splits "Street 1, 3000 City" into its street and city.
func addressLines(address string) (string, string) {
	street, city, _ := strings.Cut(address, ", ")
	return street, city
}

// parkingClicked records a parking link.
func parkingClicked(p content.Place) string {
	return track("parking_clicked", map[string]string{"placement": "contact_section", "parking_name": p.Name, "link_url": p.Link})
}

// lightboxOpen is a gallery photo's expression: open the lightbox on it.
// Safari does not focus clicked buttons. Focus the opener first so the
// native dialog restores it, and focus Close after the dialog is rendered.
func lightboxOpen(i int) string {
	return "$lightbox = " + strconv.Itoa(i) +
		"; el.focus({preventScroll: true}); const dialog = document.getElementById('gallery-lightbox');" +
		" dialog.showModal(); requestAnimationFrame(() => dialog.querySelector('[autofocus]').focus({preventScroll: true}))"
}

// instagramClicked records the gallery's Instagram link.
func instagramClicked() string {
	return track("instagram_clicked", map[string]string{"placement": "gallery_section", "target_type": "profile", "link_url": instagram})
}

// teamServiceName is a member's role for analytics, in English as on the
// Qwik site.
func teamServiceName(m content.Member) string {
	if m.Role != "" {
		return m.Role
	}
	return "Technician"
}
