package assets_test

import (
	"io/fs"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/image/webp"

	"github.com/lieranderl/aestheticlab/internal/assets"
	"github.com/lieranderl/aestheticlab/static"
)

// embedded is the path inside static.Files that a /static/ URL serves.
func embedded(t *testing.T, url string) string {
	t.Helper()
	path, ok := strings.CutPrefix(url, "/static/")
	if !ok {
		t.Fatalf("%s is not under /static/", url)
	}
	return path
}

func requireBuilt(t *testing.T) {
	t.Helper()
	if assets.URL("app.css") == "" {
		t.Fatal("the manifest has no app.css: run bun run assets")
	}
	if _, err := fs.Stat(static.Files, embedded(t, assets.URL("app.css"))); err != nil {
		t.Fatalf("%s is not embedded: run bun run assets, then go test (%v)", assets.URL("app.css"), err)
	}
}

// Every URL the manifest gives is served from the binary, under a hashed
// name internal/server caches for a year.
func TestEveryManifestFileIsEmbeddedUnderAHashedName(t *testing.T) {
	requireBuilt(t)
	hashed := regexp.MustCompile(`\.[0-9a-f]{10}\.[a-z0-9]+$`)
	fileNames, _ := assets.Names()
	for _, name := range append(fileNames, "app.css", "datastar.js") {
		url := assets.URL(name)
		if !hashed.MatchString(url) {
			t.Errorf("%s → %q is not a hashed name", name, url)
			continue
		}
		if _, err := fs.Stat(static.Files, embedded(t, url)); err != nil {
			t.Errorf("%s → %s is not embedded: %v", name, url, err)
		}
	}
}

// Each variant decodes to the width its srcset entry claims, and the
// largest to the Width and Height the <img> attributes will carry.
func TestEveryImageVariantHasTheSizeTheManifestGives(t *testing.T) {
	requireBuilt(t)
	_, imageNames := assets.Names()
	if len(imageNames) == 0 {
		t.Fatal("the manifest has no images")
	}
	for _, name := range imageNames {
		img, _ := assets.Lookup(name)
		for entry := range strings.SplitSeq(img.SrcSet, ", ") {
			url, w, ok := strings.Cut(entry, " ")
			if !ok || !strings.HasSuffix(w, "w") {
				t.Errorf("%s: srcset entry %q", name, entry)
				continue
			}
			want, _ := strconv.Atoi(strings.TrimSuffix(w, "w"))
			f, err := static.Files.Open(embedded(t, url))
			if err != nil {
				t.Errorf("%s: %s is not embedded", name, url)
				continue
			}
			cfg, err := webp.DecodeConfig(f)
			_ = f.Close()
			if err != nil {
				t.Errorf("%s: %s does not decode as WebP: %v", name, url, err)
				continue
			}
			if cfg.Width != want {
				t.Errorf("%s: %s is %dpx wide, srcset says %d", name, url, cfg.Width, want)
			}
			if url == img.Src && (cfg.Width != img.Width || cfg.Height != img.Height) {
				t.Errorf("%s: largest variant is %dx%d, manifest says %dx%d", name, cfg.Width, cfg.Height, img.Width, img.Height)
			}
		}
	}
}

// Byte-identical sources share one set of files, so a page never makes the
// browser fetch the same picture twice under two names.
func TestByteIdenticalSourcesShareTheirVariants(t *testing.T) {
	requireBuilt(t)
	legacy, ok1 := assets.Lookup("gallery/lazer1.jpg")
	canonical, ok2 := assets.Lookup("gallery/universal.jpg")
	if !ok1 || !ok2 {
		t.Fatal("gallery/lazer1.jpg or gallery/universal.jpg is missing from the manifest")
	}
	if legacy != canonical {
		t.Errorf("lazer1.jpg and universal.jpg have the same bytes but different variants:\n%v\n%v", legacy, canonical)
	}
}

// White-shimmer.jpg is stored sideways with an EXIF rotation; the Qwik site
// renders it upright (1200x1600), and so must the variants.
func TestImagesFollowTheirEXIFOrientation(t *testing.T) {
	img, ok := assets.Lookup("gallery/white-shimmer.jpg")
	if !ok {
		t.Fatal("gallery/white-shimmer.jpg is missing from the manifest")
	}
	if img.Width != 1200 || img.Height != 1600 {
		t.Errorf("white-shimmer.jpg is %dx%d, want 1200x1600 (upright)", img.Width, img.Height)
	}
}

// The site is light only: every visitor gets the Aesthetic theme, whatever
// their system colour scheme. (The Qwik site also shipped DaisyUI's dark
// theme, so dark-mode visitors got its base colours under the sage palette.)
func TestTheStylesheetIsLightOnlyWithTheAestheticTheme(t *testing.T) {
	requireBuilt(t)
	css, err := fs.ReadFile(static.Files, embedded(t, assets.URL("app.css")))
	if err != nil {
		t.Fatal(err)
	}
	s := string(css)
	if strings.Contains(s, "prefers-color-scheme:dark") || strings.Contains(s, "prefers-color-scheme: dark") {
		t.Error("app.css switches theme in dark mode")
	}
	for _, want := range []string{
		":where(:root)",
		"[data-theme=Aesthetic]",
		"color-scheme:light",
		"--color-base-100:#f7f5ef",
		"--color-base-content:#1c211b",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("app.css lacks %s", want)
		}
	}
	// DaisyUI's built-in dark theme sets base-100 to #1d232a.
	if strings.Contains(s, "#1d232a") {
		t.Error("app.css still carries DaisyUI's dark theme")
	}
}

func TestUnknownNamesResolveToNothing(t *testing.T) {
	if got := assets.URL("nope.css"); got != "" {
		t.Errorf("URL(nope.css) = %q", got)
	}
	if _, ok := assets.Lookup("nope.jpg"); ok {
		t.Error("Lookup(nope.jpg) found an image")
	}
}
