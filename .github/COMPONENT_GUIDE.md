# Views Guide

How pages and sections are built in `internal/views`. Follow `CODING_STANDARDS.md` for the general rules.

## Layout

| File | Holds |
| --- | --- |
| `layout.templ` | `Layout`: the HTML document, SEO head, cookie banner, booking dialog |
| `chrome.templ` | Navigation, mobile menu, language switcher, phone booking bar, footer, and shared pieces: `BookButton`, `Kicker`, `Picture`, `FadeUp`, `ScrollToTop` |
| `home.templ` | The home page sections: hero, gallery and lightbox, team, FAQ, contact |
| `services.templ` | The treatments section and its Datastar fragment (`ServicesFragment`) |
| `pricelist.templ` | The price list page |
| `policies.templ` | Privacy, care notes and the branded 404 |
| `icon.templ`, `icons_gen.go` | Inline SVG icons from `make assets` |
| `*.go` beside them | View models built from `internal/content`, SEO, booking links |

Routing and HTTP concerns (locales, headers, caching, old Qwik URLs) live in `internal/server`, not here.

## Adding a Section

1. Write a test first in the matching `*_test.go`: render the component and assert on the HTML it must contain (a heading, a link, an `aria-*` attribute).
2. Add a `templ` component to the page's file. Take data as arguments (`loc *content.Localized`, a struct from the view model); never fetch.
3. Use the shared pieces: `@Kicker`, `@FadeUp`, `@Picture`, `@BookButton`, `sectionWrapper` on the home page.
4. Put every visible string through `tr`/`trf`/`trc`, add it to all five locale files, then run `make i18n` (see `I18N_GUIDE.md`).
5. Compose it in the page component, add a navigation anchor if the menu links to it, and run `make ci`.
6. If it changes a user flow, extend the matching spec in `e2e/` and run `make test-e2e`.

## Images and Media

- Source files go in `frontend/media/`. `make assets` writes WebP variants and records them in `internal/assets/manifest_gen.go`; commit that file.
- Render with `@Picture(name, alt, class, sizes, eager)`: the manifest supplies width, height and `srcset`. Use `eager` only for the first visible image.
- Byte-identical files share one canonical asset; don't add a second copy under another name.
- Videos and their posters are copied as they are; start them with `inViewVideo` so they play only in view.

## Interactive State

- Local state is a Datastar signal on the closest element (`data-signals`), changed by `data-on:*` and shown with `data-class`, `data-attr`, `data-show` or `data-text`.
- State that belongs in the URL (the selected treatment category) is a query parameter. The full page renders it, and a Datastar request gets only the patched section.
- Dialogs are native `<dialog>` elements with DaisyUI `modal`; the browser handles focus and Escape.

## Checklist

- Test written first and passing; `make ci` green.
- All strings translated in five locales; `make i18n` committed.
- Keyboard and screen-reader access checked; images sized; no layout shift.
- Works without JavaScript where it can; no CSP violations in the browser console.
