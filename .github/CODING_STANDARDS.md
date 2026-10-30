# Coding Standards

`AGENTS.md` is the policy; this guide is the detail for code changes. The `.agents/skills/` guides go deeper: `go-backend.md`, `datastar-expert.md`, `daisyui-expert.md`, the `golang-*` skills and `tdd`.

## Go

- Write the test first, at a public seam: an `httptest` request against `internal/server`, a rendered component in `internal/views`, or an exported function. Prefer table-driven tests and behaviour names (`TestAStaleCacheRefreshesInsideTheRequest`).
- `make ci` is the gate: `gofmt`, `go vet`, staticcheck, golangci-lint (`.golangci.yml`), `go test -race`, govulncheck and gosec.
- Return errors with context (`fmt.Errorf("parse %s: %w", name, err)`); don't log and return the same error. Handlers log once, with `slog` and a snake_case event name (`services_patch_failed`).
- Keep packages small and one-directional: `cmd/web` wires `internal/server`, which renders `internal/views` from `internal/content` and `internal/i18n`. Views never fetch data.
- Pass `context.Context` first. The locale and the request URL travel in it (`i18n.WithLocale`, `views.WithRequest`).
- Comments say why, in full sentences. Generated files (`*_templ.go`, `*_gen.go`) are never edited by hand.

## Templates (templ)

- Pages and sections live in `internal/views/*.templ`; run `make templ` (or `make ci`) and commit the generated `*_templ.go`.
- Every visible string goes through `tr`, `trf`, `trc` or `trJS` (see `I18N_GUIDE.md`).
- Images use `@Picture(...)` with a manifest name, so width, height and `srcset` come from `internal/assets`.
- Icons use `@Icon("outline/…", class)`; a new icon name is picked up by `make assets`.

## Interaction (Datastar)

- Interaction is declarative: `data-signals`, `data-on:*`, `data-class`, `data-attr`, `data-show`, `data-text`. No framework, no inline `<script>` handlers.
- Server updates use SSE fragments (`datastar.NewSSE(...).PatchElementTempl(...)`), sent `no-store`, for requests with `Datastar-Request: true`.
- Prefer real links and forms that Datastar enhances, so a page still works when a script fails.
- `frontend/site.js` holds only what Datastar can't do (Consent Mode, analytics, scroll-motion fallbacks). Keep it small and tested by the browser specs.

## Styling

- DaisyUI components and semantic theme tokens first (`btn`, `menu`, `modal`; `bg-base-100`, `text-base-content`), Tailwind utilities second. See `DAISYUI_PATTERNS.md`.
- The theme and custom CSS live in `frontend/app.css`. No inline styles, no `tailwind.config.js`, no raw hex colours in templates.
- Respect `prefers-reduced-motion`; scroll effects use CSS scroll-driven animation with a script fallback only where a browser lacks it.

## JavaScript, CSS, JSON and Markdown

- Biome formats and lints them (`bun run biome`; CI runs `bunx --bun biome ci .`). Do not add ESLint or Prettier.
- Build scripts in `frontend/*.mjs` have `bun test` tests beside them (`make frontend-test`).

## Design Principles

- **Keep it simple:** server-rendered HTML, small handlers, no client state that the URL could hold.
- **Secure by default:** strict CSP from `internal/server/headers.go`, no secrets in the runtime, read-only data access.
- **Fail soft:** when Firestore is slow or down the site serves its last good content, else the embedded snapshot.
- **Accessible:** semantic HTML, keyboard-reachable controls, visible focus, meaningful `alt`, labels on icon buttons.
- **Stable layout:** images carry their dimensions; fonts and media never shift content after load.
- **Mobile first:** design at 375 px, then widen.
