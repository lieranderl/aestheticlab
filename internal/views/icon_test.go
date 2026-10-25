package views_test

import (
	"context"
	"strings"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/views"
)

func render(t *testing.T, name, class string) string {
	t.Helper()
	var b strings.Builder
	if err := views.Icon(name, class).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// The Qwik site's icons render with the attributes @qwikest/icons gives
// each set, so swapping stacks does not change how they look.
func TestTheQwikSitesIconsRenderAsInlineSVG(t *testing.T) {
	for _, tc := range []struct{ name, attrs string }{
		{"outline/arrow-long-right", `viewBox="0 0 24 24" width="1em" height="1em" fill="none" stroke="currentColor" stroke-width="1.5"`},
		{"outline/arrow-up", `viewBox="0 0 24 24" width="1em" height="1em" fill="none" stroke="currentColor" stroke-width="1.5"`},
		{"outline/map-pin", `viewBox="0 0 24 24" width="1em" height="1em" fill="none" stroke="currentColor" stroke-width="1.5"`},
		{"mini/arrow-path", `viewBox="0 0 20 20" width="1em" height="1em" fill="currentColor"`},
		{"mini/pause", `viewBox="0 0 20 20" width="1em" height="1em" fill="currentColor"`},
		{"mini/play", `viewBox="0 0 20 20" width="1em" height="1em" fill="currentColor"`},
		{"brand/instagram", `viewBox="0 0 24 24" width="1em" height="1em" fill="currentColor" stroke="none" role="img"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := render(t, tc.name, "size-5 text-primary")
			if !strings.Contains(got, tc.attrs) {
				t.Errorf("svg lacks %s: %s", tc.attrs, got)
			}
			if !strings.Contains(got, `aria-hidden="true"`) || !strings.Contains(got, `class="size-5 text-primary"`) {
				t.Errorf("svg lacks aria-hidden or the class: %s", got)
			}
			if !strings.Contains(got, "<path") {
				t.Errorf("svg has no path: %s", got)
			}
		})
	}
}

func TestAnUnknownIconRendersNothing(t *testing.T) {
	if got := render(t, "outline/no-such-icon", "size-5"); got != "" {
		t.Errorf("unknown icon rendered %q", got)
	}
}
