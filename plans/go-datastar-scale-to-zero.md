# Go + Datastar Rewrite for Cloud Run Scale-to-Zero

## Goal

Run production with **zero idle Cloud Run instances**: the service scales to zero when nobody visits, and a request starts a Go + templ + Datastar container fast enough that the first visitor does not notice. This removes the 24/7 minimum-instance charge. The site keeps its current look, URLs, five locales and Consent Mode v2, and the delivery pipeline stays the same. Site content moves from Supabase to Firestore in the same GCP project.

The rewrite follows the scaffolding, conventions and agent instructions of `/Users/evfedoto/Documents/Projects/datastar-webapp` (Moviestracker), whose Cloud Run web app already scales to zero and uses Firestore.

## Decisions

| Topic | Decision | Reason |
| --- | --- | --- |
| Uptime checks | Remove both (`/healthz`, `/dependencyz`) and the alert that uses them | Their probes, every few seconds, keep an instance warm forever. Alerts fire on real traffic instead. |
| Content store | Firestore, replacing Supabase | Supabase is on the free plan, which pauses after 7 days without requests; without the uptime checks, a quiet week could pause it. Firestore never pauses, sits in the same project, and the runtime reaches it through its service account with no API key. It's the same client Moviestracker uses. |
| Cold-path fallback | Wait up to 1.5 s for fresh content, then render the last good data or the bundled snapshot | The first visitor always gets a full page. Prices may be briefly outdated if Firestore is slow. |
| Smooth scrolling | Native: `scroll-behavior: smooth` plus browser wheel smoothing; drop Lenis | Close to today without a JS exemption. Lenis's inertia glide is not replicated. |
| Repository | This repo; module path `github.com/lieranderl/aestheticlab`; rename the GitHub repo after Qwik is removed | Infra, workflows, plans and smoke tests stay. |
| Startup probe | Ship Phase 1 now on the Qwik site | Helps today's cold starts and carries over unchanged. |
| Supabase project | Leave untouched | Out of scope. The site stops reading it at cutover. It also holds unused `public.clients` and `public.bookings` data, protected by RLS with no policies. |
| Firestore databases | New named databases `site-production` and `site-staging`; leave `(default)` and `aestheticlab-income` alone | `(default)` holds the free tier but belongs to a Telegram calendar bot; `aestheticlab-income` holds accounting data. Separate databases let an IAM condition keep the site's service account away from both. Reads cost $0.33 per million in Belgium (Cloud Billing catalog, 2026-10-25), so even the modeled worst case is about $0.44/month. |
| Dark mode | Light only: DaisyUI's built-in themes off, so the Aesthetic theme applies to every visitor | The Qwik site loads DaisyUI's light and dark themes and sets no `data-theme`, so visitors in dark mode get DaisyUI's dark base colours (`base-100` `#1d232a`) under the sage palette. That's a bug, not a design. The Go site fixes it, and `TestTheStylesheetIsLightOnlyWithTheAestheticTheme` guards against a regression. The fix ships with the cutover; Phase 6 screenshots compare in light mode. |

## Success Criteria

Targets come from the Phase 0 baseline (2026-10-25). Moviestracker's Go web app on Cloud Run sets the reachable level.

- Production `run.googleapis.com/container/instance_count` reaches 0 during quiet hours. The serving revision itself must have min instances 0, not just the IaC template.
- Cloud Run `container/startup_latencies` p95 ≤ 700 ms (today 5,370 ms; Moviestracker 667 ms).
- Cold-path first-byte time for `/en-BE/` from Europe ≤ 1.2 s, including Cloud Run provisioning (today 5.58 s; Moviestracker 1.08 s).
- Warm-path behavior is no worse than today: 2xx `request_latencies` p95 ≤ 190 ms (today's value), the existing p95 latency alert (2 s) stays quiet, and Lighthouse LCP/CLS/TBT do not regress on any page.
- Visual and behavioral parity: all existing Playwright specs pass against the Go server, and screenshot comparison against the Qwik site shows no unreviewed differences.
- Every current public URL keeps working: localized pages, `robots.txt`, `sitemap.xml`, `manifest.json`, `og/*`, fonts, icons, and pre-rewrite hashed image URLs. A Qwik tab left open across the cutover recovers to the Go site instead of breaking.
- No serving or rollback-target revision references the Supabase secret once it is deleted. The runtime holds no database credentials.
- Production Firestore reads match the read model below and are monitored. The site's databases are outside the free tier, so the budget is cost, not quota.

## Scope

- Go server in this repository (`cmd/web`, `internal/…`), replacing Qwik City and Qwik Speak.
- Asset pipeline: Tailwind CSS 4 + DaisyUI 5 via the Tailwind CLI, server-rendered Lucide icons, build-time responsive images, precompressed static assets.
- i18n for `en-BE`, `nl-BE`, `fr-BE`, `ru-BE`, `uk-BE`, using English-text keys as Moviestracker does.
- Firestore content store: data model, one-off copy from Supabase, Go reader with cache and embedded snapshot, emulator-backed tests.
- Infra in `infra/`: Firestore databases and IAM, min instances, startup probe, uptime checks and dependent alerts, memory limit, staged Supabase secret removal.
- Delivery: Dockerfile, smoke test, canary skew gate, CI quality jobs (with separate Qwik and Go e2e configurations until cutover), git hooks.
- Retiring already-open Qwik clients at cutover.
- Agent instructions and guides rewritten for the Go stack.

## Non-Goals

- Visual redesign, new features or content changes, apart from the light-only fix in Decisions.
- A content admin UI; content is edited in the Firebase console.
- Moving media to Cloud Storage or Cloud CDN; this is a follow-up if cold starts triggered by asset requests matter.
- Changing, exporting or deleting anything in the Supabase project.
- Changing the canary, attestation, digest-promotion or rollback model.
- Porting Moviestracker features unrelated to a static site: tray app, TorrServer, GStreamer, desktop packaging, Windows cross-linting.

## Assumptions

- Cloud Run request-based billing (`cpu_idle = true`) stays. Min instances are billed while idle; other instances are billed only while serving requests. CPU is disabled or severely limited once a request finishes, so no work may depend on running after the response ([Cloud Run tips](https://docs.cloud.google.com/run/docs/tips/general)).
- IaC changes to the Cloud Run template create a revision with **no traffic**. `traffic` and the image are in `ignore_changes` (`infra/main.tf:363`). A template change reaches users only with the next CI deploy, which copies the template, and its promotion. For production, that means a published release.
- Revisions that bind a secret as an environment variable cannot start an instance if retrieving it fails ([Cloud Run secrets](https://docs.cloud.google.com/run/docs/configuring/services/secrets)). With scale-to-zero, every cold start retrieves it.
- Firestore's free quota applies to one database per project ([Firestore quotas](https://docs.cloud.google.com/firestore/quotas)). In this project, `(default)` already holds it, so the site's new databases are billed from the first read.
- Content is small and changes rarely. The Supabase `gettimely` schema has 1 contact, 8 service groups, 77 services and 4 staff rows. It has no edge functions or cron jobs, and since stats were last reset it shows 12 inserts and 0 updates, so edits are manual.
- The container keeps port 3000. A Go image can then be deployed as a no-traffic tagged revision of the existing staging service with no Cloud Run IaC change.
- `package.json` stays (for Tailwind, DaisyUI, Lucide, sharp and Playwright) and its `version` stays the release-tag source that `deploy.yml` validates.

## Baseline Facts From the Codebase

- `infra/variables.tf:152`: `production_min_instances` defaults to `1`. Staging is `0`.
- `infra/main.tf:399` and `:426`: uptime checks on `/healthz` every 60 s and on `/dependencyz` every 300 s, from EUROPE, USA and ASIA_PACIFIC. The `localized_page` alert policy (`:484`) depends on both.
- `infra/main.tf:334`: the startup probe uses `period_seconds = 5`, so a container that misses the first probe waits 5 s more.
- The container has 1 vCPU and 1 GiB, `startup_cpu_boost = true`, concurrency 40, `session_affinity = true`. Production `max_instances` is 3.
- The runtime image is Bun distroless plus production `node_modules`, `dist/` and `server/`. Moviestracker's is a single static binary on `distroless/static`.
- Qwik serves hashed chunks under `/build/`, images under `/assets/`, and SPA route data as `…/q-data.json`. It registers a service worker (`src/routes/service-worker.ts`) that returning visitors still run.
- `src/shared/legacy-image-request.ts` already preserves pre-v4.0.1 image URLs. Search engines and shared links also hold current `/assets/<hash>-<name>.webp` URLs.
- The production canary gate (`deploy.yml:399-435`) counts previous-revision 404s on `/build/` and `/assets/` paths that the candidate serves, and rolls back above `CANARY_MAX_ASSET_404` (5). It assumes both revisions are the same stack.
- `quality.yml` runs Playwright against `bun run dev` with `e2e/mock-supabase.ts` (`playwright.config.ts`).
- Source media is about 15 MB: gallery, services, hero, team photos and five MP4 clips.
- Supabase project `aestheticlab` is in eu-central-1 on the free plan. Anon RLS policies expose only active services and staff, so a copy made with the publishable key would miss inactive rows.

## Phase 0 Results (2026-10-25)

Measured with the old probe (`period_seconds = 5`) still serving. Metrics are Cloud Run's 7-day distributions, merged across series. Cloud Run reports percentiles from histogram buckets, so values that land in the same bucket can come out identical.

| Measure | Staging (Qwik) | Production (Qwik) | Moviestracker web (Go) |
| --- | --- | --- | --- |
| `startup_latencies` p50 / p95 / p99 | 5,149 / 5,370 / 5,389 ms | 5,149 / 5,370 / 5,389 ms | 513 / 667 / 826 ms |
| Cold first byte, single request | 5.58 s (idle ~50 min) | — (min 1, never cold) | 1.08 s (idle 79 min) |
| Warm first byte, `/en-BE/` | 0.14–0.23 s | — | 0.045 s |
| 2xx `request_latencies` p95 | 165 ms | 190 ms | 372 ms |
| Memory utilization p99 (of 1 GiB) | 3% | 5% (~54 MB) | 9% |
| Requests, 7 days | 2,435 | 52,487 | 7,881 |
| Active instances, hourly max | min 0 / mean 0.79 / max 2 | min 1 / mean 1.03 / max 2 | min 0 / mean 0.99 / max 3 |
| Image, compressed | 56.9 MB (21 layers) | 56.9 MB | 9.0 MB (13 layers) |
| HTML, `/en-BE/` | 172 KB | — | — |

- **Startup is one probe period.** Startup latency sits at about 5.1 s, almost exactly the 5 s probe interval: the first probe misses and the next one is 5 s later. Phase 1 (probe every 1 s) should remove most of it on its own. Re-measure once the new probe is on a serving revision.
- **Production traffic is mostly synthetic.** At about 7.5k requests/day, the uptime checks account for roughly 5.2k (`/healthz` 1/min and `/dependencyz` 1/5 min from three regions). Real traffic, including assets, is around 2.3k requests/day. That's why one instance is always active.
- **Memory headroom is large.** Qwik peaks near 54 MB, so 256 MiB is safe even before the Go rewrite.
- **Firestore is already in use.** `(default)` (europe-west1, free tier, created 2026-04-27) holds `calendar_telegram_*` collections from a Telegram calendar bot. `aestheticlab-income` (europe-west1, created 2026-10-08) holds an accounting app's `users`, `accounts`, `ledger`, `audit` and related collections. Neither is managed in `infra/`. Only collection names were listed; no documents were read.
- **Lighthouse and screenshots are deferred.** PageSpeed Insights' shared daily quota was exhausted. Since Qwik stays in the repo until Phase 10, both are captured side by side from local builds of each stack in Phase 6.

## Target Architecture

### Repository Layout

Modeled on Moviestracker. During migration it lives next to `src/`, which it does not overlap.

| Path | Owns |
| --- | --- |
| `cmd/web/main.go` | Config from env, logger, server start. No network I/O before `Listen`. |
| `cmd/content-import/` | One-off Supabase-export → Firestore copy, run by a human with their own credentials. |
| `internal/server/` | Router, middleware (security headers, compression, cache headers, logging), `/healthz`, `/readyz`, `/dependencyz`, compatibility redirects, the Qwik-client shim. |
| `internal/content/` | Firestore reader, document parsing and per-locale projection, cache, embedded snapshot. |
| `internal/i18n/` | Locales, URL locale resolution, translation tables for 5 locales, plural rules. |
| `internal/views/` | templ layouts, sections and UI primitives (the counterpart of `src/components/{sections,ui}`). |
| `internal/assets/` | Generated image manifest and helpers (`@Picture`, hashed asset URLs). |
| `frontend/` | `app.css` (DaisyUI theme moved unchanged from `src/global.css`), icon and image generators, the consent bootstrap. |
| `static/` | Built CSS, JS and image variants, vendored `datastar.js`, fonts, public files. Embedded with `go:embed` and served under `/static/`, never under Qwik's `/build/` or `/assets/`. |
| `e2e/go/` | Go-side Playwright config and Firestore emulator seed, alongside the Qwik config until Phase 10. |
| `docs/CONTENT.md` | Firestore data model and how to edit content in the console. |

Pinned toolchain, as in Moviestracker: Go 1.27.x, `github.com/a-h/templ`, `github.com/starfederation/datastar-go`, `cloud.google.com/go/firestore`, `golang.org/x/sync`, and `go tool` entries for templ, golangci-lint, staticcheck, gosec and govulncheck. Tailwind and DaisyUI versions are pinned exactly in `package.json`.

### Content on Firestore

- **Databases:** `site-production` and `site-staging`, Native mode in `europe-west1`, managed in OpenTofu with delete protection. Both are billed per read, since `(default)` holds the project's free tier.
- **Neighbors:** `(default)` (Telegram calendar bot) and `aestheticlab-income` (accounting data) are not the site's. OpenTofu must not manage, import or modify them. The site's identities never get an unconditioned `datastore` role, because a project-wide grant would expose the accounting data.
- **Data model:** keep Supabase column names as field names (`name`, `name_en`, `name_nl`, `name_fr`, `name_ru`, `name_uk`, `priority`, `active`, `price`, `duration`, …) so parsing and projection port directly.
  - Collections: `service_groups`, `services`, `staff`.
  - Single document: `site/contact`.
  - Document IDs: the current Supabase IDs.
- **Access:** each environment's runtime service account gets read-only `roles/datastore.viewer`, limited to its own database with an IAM condition. Content editors get write access through IaC-managed membership, never console-only grants (see open questions).
- **Copy:** export all rows of the four `gettimely` tables, inactive ones included, as JSON with a SQL query run by a human through Supabase's own tooling. No service-role key goes into the repo or CI. `cmd/content-import` loads that JSON into Firestore with the operator's application-default credentials. The export files stay out of git.
- **Dual-source window:** until the production cutover, Qwik reads Supabase and Go reads Firestore. Freeze content edits from the first import until release, or re-run the import immediately before the production release.

### Cold-Start Design

1. **Listen first.** `main` reads env, builds handlers from embedded data and calls `ListenAndServe` right away. It creates the Firestore client without waiting on the network, and starts one content prefetch while the instance is still starting, when CPU is allocated. The first request joins that prefetch instead of starting its own.
2. **Nothing to parse at boot.** templ compiles templates to Go, translations are Go maps, the image manifest is generated Go, and static files are `go:embed`. Startup is process exec plus a few allocations.
3. **Small image.** Use `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w"` on `gcr.io/distroless/static-debian12:nonroot`, pinned by digest. Record image size in Phase 2 and Phase 8. Media dominates it, so generate only the variant widths the layout uses.
4. **Content refresh happens inside requests.** After startup, no content work is scheduled outside a request handler.
   - Fetch all four collections in parallel (`errgroup`) and project per locale in memory. One fetch serves all five locales; today each locale queries separately.
   - Each instance caches content in memory with a **15 min TTL**.
   - A request that finds content stale or missing starts the refresh through `singleflight`, with a detached context and its own 5 s timeout, and waits for it inside the handler:
     - up to **300 ms** when stale content exists;
     - up to **1.5 s** when none exists, as on a cold instance.
   - It then renders the freshest data available: the refresh result, else the last good data, else the **embedded snapshot** (`internal/content/snapshot.json`).
   - A refresh that outlives its request may be suspended. The next request resumes it and joins it through `singleflight`; if its timeout expired while suspended, that request starts a new one. No correctness depends on suspended work finishing.
   - Rendering from the snapshot logs `content_snapshot_served`, and the content-failure log metric counts that event.
   - `make snapshot` refreshes the snapshot from production Firestore with the developer's application-default credentials. A test checks that the snapshot parses and covers every locale.
   - Content edits reach visitors within about 15 minutes (TTL), plus the 60 s HTML cache. `docs/CONTENT.md` says so.
   - Measure the Firestore first-call cost (gRPC channel, TLS, metadata-server token) in Phase 5 against the 1.5 s deadline.
5. **Precompressed static assets.** CSS, JS and SVG are built with `.br` and `.gz` siblings and chosen by `Accept-Encoding`. Hashed file names get `Cache-Control: public, max-age=31536000, immutable`, so returning visitors rarely reach a cold instance for assets. HTML keeps `max-age=60, stale-while-revalidate=600`, with gzip done on the fly.
6. **Right-sized instance.** Measure peak RSS under a short load test, then lower memory from 1 GiB, likely to 256 MiB. Keep 1 vCPU and `startup_cpu_boost`. Revisit concurrency (40) from the load test.
7. **Fast startup probe.** `period_seconds = 1` and `timeout_seconds = 1` on `/healthz` (process-only), with `failure_threshold = 60`, keeping the 60 s startup budget.

### Firestore Read Model (Production)

Cache and `singleflight` are per instance, so reads scale with how many instances are active. Firestore bills one read per document returned, and a full refresh returns about 90.

| Source | Assumption | Reads/day |
| --- | --- | --- |
| TTL refreshes, worst case | 3 instances (`max_instances`) plus 1 canary candidate, each busy around the clock: 96 refreshes × 90 documents × 4 | 34,560 |
| Cold starts | Each new instance fetches once: budget 100 starts/day × 90 | 9,000 |
| `/dependencyz` and smoke tests | One bounded read per probe, a few dozen per deploy | < 500 |
| **Worst case total** | | **≈ 44,000 (≈ $0.44/month)** |
| Realistic | About 2.3k real requests/day (Phase 0), mostly zero or one instance | < 10,000 (≈ $0.10/month) |

- At $0.33 per million reads, cost is negligible at any plausible traffic. The 40k/day alert below catches a bug, such as a refresh loop or a broken cache, not normal growth.
- If reads ever matter, raise the TTL or publish the content as one document, which costs one read per refresh.
- Staging's database is billed the same way, at staging traffic.

### Frontend Mapping (Datastar First, DaisyUI First)

| Today | Go/Datastar |
| --- | --- |
| Navigation drawer, language switcher | DaisyUI `drawer`/`dropdown` with Datastar signals; language links rendered server-side for the current path. |
| FAQ, expandable text | DaisyUI `collapse` / native `<details>`; `data-class` for expand state. |
| Booking modal (Timely iframe) | Native `<dialog>` via a `modalDialog` helper; the iframe `src` is set on open with `data-attr:src` so Timely loads only when used. |
| Gallery lightbox | Native `<dialog>`; image index in a signal. |
| Fade-up, in-view video | `data-on-intersect` toggling classes and `play()`/`pause()`; honor `prefers-reduced-motion`. |
| Parallax | CSS scroll-driven animation (`animation-timeline: view()`) with a static fallback; no JS. |
| Smooth scrolling (Lenis) | `scroll-behavior: smooth` on `html` (disabled under `prefers-reduced-motion`) and `scroll-margin-top` on anchor targets. The wheel uses the browser's own smoothing. |
| Cookie banner + Consent Mode v2 | Datastar signals persisted to `localStorage`; gtag bootstrap and consent defaults move to an external `static/consent.js`, loaded before gtag, so CSP needs no inline script. |
| Scroll-to-top | Datastar `data-on:scroll__window` signal plus a button using native smooth scrolling. |

CSP target: `script-src 'self' 'unsafe-eval' https://www.googletagmanager.com` (Datastar evaluates expressions) and `style-src 'self'`, with no inline `style=""` attributes. Today's policy allows `'unsafe-inline'` for both. Check GA and Timely in a browser before tightening further. The Firestore client runs on the server, so CSP `connect-src` needs no change for it.

### Retiring Open Qwik Clients

A Qwik tab opened before the cutover keeps running Qwik code. On interaction it lazy-loads `/build/*.js` chunks, and on in-app navigation it fetches `…/q-data.json`. After promotion those requests reach Go.

- **Chunks:** Go answers any `GET /build/*.js` with a tiny JavaScript module whose body calls `location.reload()`, sent with `Cache-Control: no-store`. Qwik loads chunks with dynamic `import()`, so the module body runs and the tab reloads into the Go page.
- **Route data:** for `…/q-data.json`, return the response that makes Qwik City fall back to a full page load. Determine which response that is (404 or a redirect) with the test below.
- **Service worker:** `/service-worker.js` serves a kill switch that unregisters itself and clears its caches.
- **No path collisions:** Go assets live under `/static/`, and `/assets/` only redirects old image URLs. Neither stack's hashed paths can resolve to the wrong stack's content.
- **Canary gate:** for the cross-stack v5.0.0 release, revise the skew check (`deploy.yml:399-435`) and `.github/DEPLOYMENT.md`:
  - Qwik `/build/` requests that reach the Go candidate get the shim's 200, so they don't count as skew.
  - Go `/static/` requests that reach the Qwik revision 404, and must be counted as skew.
- **Test:** a Playwright spec behind a small test proxy. It loads a Qwik page, switches the proxy's upstream to the Go server, then clicks an interactive element and follows an in-app link. The tab must end on the Go page with no unhandled console errors. Run it in CI until Phase 10.

### Infra Changes

| Resource | Change | When |
| --- | --- | --- |
| `startup_probe` | `period_seconds` 5 → 1, `timeout_seconds` 3 → 1, `failure_threshold` 12 → 60 (same 60 s budget) | Phase 1 |
| `google_project_service` | Declare `firestore.googleapis.com` (already enabled for the neighboring apps; `disable_on_destroy = false` as for the other APIs) | Phase 5 |
| `google_firestore_database` | `site-production` and `site-staging`, Native mode, `europe-west1`, delete protection | Phase 5 |
| Runtime IAM | `roles/datastore.viewer` per environment, conditioned on its own database | Phase 5 |
| Cloud Run env | Add `FIRESTORE_DATABASE` (and `GOOGLE_CLOUD_PROJECT` if not inferred) | Phase 8 |
| Cloud Run env | Remove `SUPABASE_URL` and the `SUPABASE_KEY` secret binding | Phase 9b |
| `google_secret_manager_secret.supabase_key` and runtime IAM | Remove | Phase 9c, after the Qwik rollback window closes |
| `google_logging_metric.supabase_failures` and its alert | Rename to content failures; match Firestore read errors and `content_snapshot_served` | Phase 9a |
| New alert policies | `container/startup_latencies` p95 above target for 15 min; Firestore reads > 40k/day | Phase 9a |
| `production_min_instances` | `1` → `0` | Phase 9a |
| `google_monitoring_uptime_check_config.localized_page`, `.supabase_dependency` | Remove | Phase 9a |
| `google_monitoring_alert_policy.localized_page` | Remove; the 5xx, latency, runtime-failure and content-failure alerts remain | Phase 9a |
| `resources.limits.memory` | `1Gi` → measured value (likely `256Mi`) | Phase 9a |
| `container_port` / probe ports | Unchanged (3000) | — |

- **Template changes need a deploy.** Every Cloud Run template change takes effect only after a CI deploy copies the template and promotes the new revision: a merge to `staging` for staging, a published release for production. Each phase with template changes includes that deploy before anything is measured or checked.
- **Accepted tradeoff:** with no uptime checks, there is no signal when the site is broken *and* nobody is visiting. Deploy smoke tests still exercise `/readyz` and `/dependencyz` on every release.
- All infra changes go through a saved plan in the protected `infrastructure` workflow, per `AGENTS.md`. No console changes.

## Phases

### Phase 0: Baseline (Done 2026-10-25)

Results are in "Phase 0 Results" above. Done:

- staging cold and warm requests;
- 7-day startup and request latency, memory, request and instance metrics for staging, production and `moviestracker-web`;
- image sizes;
- the Firestore database list;
- final targets and the read model.

The 7-day startup distribution replaced the planned five spaced cold requests: it covers every cold start of the week, not five samples. Lighthouse and screenshots moved to Phase 6.

### Phase 1: Probe Quick Win (Qwik, Now)

1. Change the startup probe to `period_seconds = 1`, `timeout_seconds = 1`, and `failure_threshold = 60`, keeping today's 60 s startup budget (12 × 5 s).
2. Merge through the normal PR to `staging`, then apply through the infrastructure workflow.
3. Deploy and promote so the change reaches serving revisions: a merge to `staging` for staging, the next published release for production.
4. Re-measure staging cold starts and `startup_latencies`. Expected: well under 1 s, against 5.1 s at Phase 0.

Status:

- Merged in [#235](https://github.com/lieranderl/qwik-aestheticlab/pull/235) as `757bcdc`; applied by infrastructure run 40195855239 (0 added, 2 changed, 0 destroyed; only the probe fields). The [#237](https://github.com/lieranderl/qwik-aestheticlab/pull/237) staging deploy copied the template, so staging's serving revision `stage-aestheticlab-web-candidate-40199687517-1` probes every 1 s. Production picks it up with the next release.
- Measured 2026-10-25 14:25 UTC on staging after 18+ min idle, with a new instance confirmed in Cloud Run logs: cold first byte 1.68 s (was 5.58 s), Cloud Run request latency 1.53 s, warm 0.14–0.18 s. The instance went from start to ready in about 1.1 s (was about 5.1 s): `Starting new instance` 14:25:58.32 → Bun `server_started` +0.72 s → probe passed on attempt 2, +0.37 s.
- Gate met. Qwik still misses the Go targets (startup p95 ≤ 700 ms, cold first byte ≤ 1.2 s) mainly because of Bun's 0.7 s boot.

### Phase 2: Go Scaffold

1. `go.mod` (`github.com/lieranderl/aestheticlab`), `cmd/web`, `internal/server` with health endpoints, security headers, structured JSON logging matching today's event names, and an embedded static handler with range support (MP4 seeking).
2. Copy and adapt from Moviestracker: `.golangci.yml`, `.gitleaks.toml`, `.agents/skills/` (Go, Datastar, DaisyUI, TDD), `docs/` guide structure, Makefile targets (`templ`, `templ-check`, `assets`, `assets-check`, `lint`, `test`, `security`, `ci`, `dev` with Air, `firestore` emulator, `snapshot`).
3. `Dockerfile.web` (Bun assets stage → Go build stage → distroless). The existing `Dockerfile` stays Qwik until cutover. It can't be named `Dockerfile.go`: the Go tool would parse it as Go source in the module root and fail `go vet ./...`.
4. Add a Go quality job to `quality.yml` (lint, `go test -race`, govulncheck, gosec, templ/assets diff checks, Firestore emulator tests). Exclude the Go directories in `biome.json`. The existing Qwik jobs, including Qwik e2e, stay unchanged.

Status (2026-10-25): scaffold on `feat/go-scaffold`.

- `cmd/web` serves `/healthz`, `/readyz` and a `/dependencyz` stub (200 until Phase 5), the security headers with the CSP target, the `/static/` handler (ranges, a year's `immutable` caching for `<name>.<8+ hex>.<ext>` names, an hour for the rest, no folder listings), and Cloud Logging JSON (`severity`, `message`): `server_started`, `server_stopping`, `server_stopped`, `http_request` and `http_panic`.
- The Qwik Makefile keeps `dev`, `lint` and `build`, so the Go targets are `go-dev`, `go-lint` and `go-build` until Phase 10. `make ci` runs `templ-check`, `go-lint`, `test`, `security` and `go-build`. `assets`, `assets-check`, `firestore` and `snapshot` are stubs that fail with a pointer to Phases 3 and 5.
- The CI job runs `make ci`. The assets stage, assets diff check and emulator tests come with Phases 3 and 5.
- Measured on an Apple M-series Mac:

  | Measure | Go scaffold | Qwik (Phase 0) |
  | --- | --- | --- |
  | Process exec to first `/healthz` 200, local binary (40 runs) | median 4.9 ms, p95 6.5 ms (first run 217 ms, macOS first-exec check) | — |
  | `container run` to first 200, Apple `container` CLI (5 runs, includes its per-container VM boot) | 851–1,079 ms | — |
  | Image, compressed (`linux/amd64`) | 3.75 MB (13 layers) | 56.9 MB (21 layers) |
  | Binary (`darwin/arm64`, `-s -w`) | 6.7 MB | — |

### Phase 3: Assets

1. `frontend/app.css`: move the DaisyUI theme and authored utilities from `src/global.css` unchanged; build with the Tailwind CLI and keep `custom-container`.
2. Self-host Onest Variable from `@fontsource-variable/onest` under `static/fonts/`.
3. `frontend/icons.mjs` → `internal/views/icons_gen.go` for the icons the templates use, as in Moviestracker.
4. `frontend/images.mjs` (sharp): AVIF and WebP variants at the widths the layout uses, content-hashed names under `/static/`, and the generated `internal/assets/manifest_gen.go` with width, height and srcset per image. Byte-identical sources map to one canonical entry, keeping the existing invariant.
5. Precompression of text assets. Vendor `datastar.js` at a pinned version.

Status (2026-10-25): on `feat/go-assets`.

- **One build, `bun run assets`** (`frontend/icons.mjs`, then `frontend/build.mjs`), writes content-hashed files to `static/dist/`. They are embedded with `go:embed all:dist` and served under `/static/dist/`.
- **What is committed:** only the generated Go (`internal/assets/manifest_gen.go`, `internal/views/icons_gen.go`). Hashes come from source bytes and encoder settings, never from encoder output, so every machine writes the same manifest. `make assets-check` fails when it is stale, and `make ci` runs it first. The CI `go` job and `Dockerfile.web` (new Bun assets stage) build the assets themselves.
- **CSS:** `frontend/app.css` is `src/global.css` unchanged, except:
  - fonts load from `/static/` with hashed names;
  - the Lenis import is gone, replaced by `scroll-behavior: smooth` outside reduced motion;
  - only templates (`internal/**/*.templ`) and the view helpers (`internal/views/*.go`) are scanned, with tests and generated Go left out; otherwise words in texts, test fixtures and SVG paths become DaisyUI CSS.

  Today it's 14.0 KB, before any page classes; scanning all Go files had produced 20.3 KB of mostly unused rules. `custom-container` isn't defined anywhere in `src/global.css`, so there was nothing to carry over.
- **Dark mode:** DaisyUI's built-in themes are off (`themes: false`), so the Aesthetic theme is the only one, with `color-scheme: light`. This fixes the Qwik site's dark-mode bug; see Decisions.
- **Fonts:** Onest's seven subsets and `public/fonts` are copied with hashed names. The three Cormorant Garamond weights are byte-identical files, so they share two URLs, per the byte-identical rule.
- **Images match Qwik City's own settings,** read from its vite-imagetools config:
  - WebP at quality 75 and widths 200/400/600/800/1200, never enlarged;
  - EXIF orientation applied (`white-shimmer.jpg` renders upright at 1200×1600);
  - no AVIF, a deliberate change from step 4.

  72 of the 104 variants the live pages use are byte-identical to Qwik's; the rest differ by at most 1 px of height rounding and under 2% in size. `lazer1.jpg` and `universal.jpg` share one variant set. Videos, posters and SVGs are copied with hashed names.
- **Icons:** Heroicons outline and mini plus Simple Icons, not Lucide. These are the sets the Qwik site renders through `@qwikest/icons`, and the `Icon` templ component copies their 1em attributes. The seven icons Qwik uses today are always generated.
- **Compression:** CSS, JS and SVG get `.br` and `.gz` siblings. `internal/server` serves the best one the browser accepts (honouring `q=0`), with `Vary: Accept-Encoding`. Datastar v1.0.4 is vendored in `frontend/vendor/datastar.js`, the same build Moviestracker pins.
- **Media location:** sources still come from `src/media` and `public/fonts`. Phase 10 moves them when `src/` goes away.
- **Measured:**
  - full build 7.7 s, rebuild 0.5 s;
  - 234 built files, 12 MB;
  - binary 19.3 MB (from 6.7 MB);
  - image about 14.2 MB compressed (arm64, Apple `container`);
  - `container run` to first 200 in 1,040 ms, unchanged from Phase 2's 851–1,079 ms.

### Phase 4: i18n

1. One-off converter from `i18n/<locale>/{app,runtime}.json` (`key@@Default`) to Go tables keyed by English text. Afterwards Go is the source of truth and the JSON files are deleted in Phase 10.
2. `tr`, `trf`, `trn`, and `trJS` (for Datastar expressions), with ru/uk plural rules.
3. Locale routing that matches today's: `/` → 302 `/en-BE/`; an unknown path keeps the locale of its first segment for the 404 page; trailing slashes as now.
4. Test: every interface text has a translation in all four non-English locales, as Moviestracker's `TestEveryInterfaceTextHasARussianTranslation` does.

Status (2026-10-25): on `feat/go-i18n`.

- **Converter:** `cmd/i18n-convert` (Go) writes `internal/i18n/messages_gen.go` from `i18n/*/app.json`, through `go/format`.
  - It keeps only the 227 keys `src/` uses, as 223 distinct English texts; the other 134 keys in the files are stale and are dropped.
  - `runtime.json` is empty in every locale.
- **The English comes from `i18n/en-BE/app.json`, not the `@@Default` texts in code.** The site renders the JSON, and 40 code defaults are out of date (for example "Hours" where the site shows "Opening hours").
- **Change from step 1:** the JSON stays the source until Phase 10, while the Qwik site is still the one being edited. `make i18n-check` (part of `make ci`) fails when the Go tables are stale. Go becomes the hand-edited source only when the JSON is deleted.
- **Keys are English text,** as in Moviestracker. Two English texts are translated differently by where they appear. They get explicit contexts, through `i18n.C` and `trc`:
  - "Open navigation menu" means the menu button in one place and the navigation landmark in another;
  - "Back to categories" means the price list link in one place and the treatment overview in another.

  The converter fails on any new collision, so a new one needs a decision.
- **Placeholders:** `{{name}}` becomes `%v`, or `%[1]v`, `%[2]v` when a text has several. Every translation keeps the English order, and the converter checks that each translation has the same placeholders.
- **No plurals (change from step 2):** no text needs plural forms. The one count, Google reviews, is phrased in ru/uk to avoid them ("Отзывов в Google: N"). So there's no `trn` yet; add it, with ru/uk rules, when a text needs one.
- **Template helpers:** `internal/views` has `tr`, `trf`, `trc`, `trJS` and `pageLang`.
- **Routing (`internal/server`) matches staging's measured behaviour:**
  - `/` returns 302 to `/en-BE/`.
  - A page path without its trailing slash gets a 301, query kept; file-like paths (`/favicon.svg`) don't.
  - Locale codes are case-sensitive (`/en-be/` is English).
  - Unknown pages are 404s in the path's locale, with `Content-Language` and `no-store`.
  - The slash redirect never builds `//host` or `/\host` Locations.

  The 404 body is a localized placeholder until Phase 6's branded page.
- **Completeness test:** `TestEveryInterfaceTextHasATranslationInEveryLocale` scans templates and handlers for translated texts. Its minimum match count, 2 today (the 404 texts), rises as Phase 6 adds pages. `i18n` also checks every generated text in every locale.

### Phase 5: Content on Firestore

1. IaC through the infrastructure workflow: declare the Firestore API, create `site-production` and `site-staging`, and add runtime IAM conditioned on each database. Review the saved plan to confirm it doesn't touch `(default)` or `aestheticlab-income`.
2. Write `docs/CONTENT.md` (data model, field meanings, how to edit in the console, the ~15 min propagation delay).
3. `cmd/content-import`: load an exported JSON into a target database idempotently (upsert by ID). Seed staging first, then production.
4. Port parsing and projection from `src/shared/{supabase-data,locale-content,service-utils,service-catalog}.ts` and their specs as table-driven Go tests over Firestore documents.
5. Reader, per-instance cache, `singleflight`, the in-request waits (300 ms stale, 1.5 s cold), snapshot fallback and `/dependencyz` (one bounded Firestore read).
6. Emulator tests (`make firestore`, as in Moviestracker):
   - cold fetch within the deadline;
   - slow or failing emulator → snapshot;
   - error → last good;
   - concurrent requests → one fetch per collection;
   - stale cache → refresh completes inside a request;
   - an abandoned refresh is joined or replaced by the next request.
7. Add `e2e/go/playwright.config.ts`: the Go server plus a Firestore emulator seeded from the same fixture data as `e2e/mock-supabase.ts`. Leave `playwright.config.ts` and the mock untouched, since they still test the Qwik site that CI ships.

### Phase 6: Pages and Sections

1. Layout, `<head>` and SEO metadata per page (port `e2e/seo-head.spec.ts` expectations), Consent Mode v2 and the cookie banner.
2. Pricelist first: it covers content, i18n and images together.
3. Home sections (hero with video, services, gallery, team, FAQ, contact, footer), policies, 404.
4. Run the shared Playwright specs under the Go config, and add that run to CI next to the Qwik run.
5. Capture screenshots (every page × 5 locales × mobile/desktop) and Lighthouse LCP/CLS/TBT for `/en-BE/` and `/en-BE/pricelist/` from local production builds of both stacks, side by side and under the same conditions. Review every difference.

### Phase 7: Compatibility

1. The open-Qwik-client shim for `/build/*.js` and `…/q-data.json`, the service-worker kill switch, and the open-tab proxy spec, as described above.
2. 301 redirects from current and legacy hashed `/assets/<hash>-<name>.<ext>` URLs to the canonical new variant, covering `legacy-image-request.ts` cases.
3. Same paths for `robots.txt`, `sitemap.xml`, `manifest.json`, `favicon.svg`, icons and `og/*`.
4. Revise the canary skew gate and `.github/DEPLOYMENT.md` for the cross-stack release.

### Phase 8: Cutover on Staging

1. Add `FIRESTORE_DATABASE` to the Cloud Run template through IaC. Supabase env stays in the template for now, so a CI deploy of either image still starts.
2. Swap `Dockerfile` to the Go build. `deploy.yml` stays build-once, attest, scan, and digest-promote.
3. Switch CI's primary e2e job to the Go config. Keep the Qwik e2e job and the open-tab spec until Phase 10.
4. Update `scripts/smoke-deployment.sh` to collect image URLs from the rendered HTML, keeping cookie-jar session affinity across page and asset probes.
5. Before merging, deploy the Go image as a no-traffic tagged revision on staging for a preview URL.
6. Merge, let the staging deploy run (it copies the Phase 8 template), and measure staging cold starts against the targets.

### Phase 9: Production and Scale-to-Zero

#### 9a: Release and Scale to Zero

1. Re-run the content import into production (or confirm the edit freeze held), then release `v5.0.0` through the normal canary path with the revised skew gate. Rollback to the last Qwik revision stays available.
2. After a week of normal alerts, apply through the protected infrastructure workflow for both environments:
   - min instances 0;
   - uptime checks and their alert removed;
   - startup-latency and Firestore-read alerts added;
   - content-failure metric renamed;
   - memory lowered.
3. Deploy and promote so the serving revisions pick up the new template: a merge to `staging`, then a patch release (`v5.0.1`) through the canary path.
4. Measure only after that promotion. Confirm the serving revision shows min instances 0, `instance_count` reaches 0 overnight, cold-start metrics meet the targets, and Firestore reads match the model.

#### 9b: Drop the Supabase Binding

1. Apply a template without `SUPABASE_URL` and the `SUPABASE_KEY` binding.
2. Deploy and promote a Go revision built from it.
3. From here on, rollback targets are this revision or a later one. Older Go revisions still reference the secret, which is fine while it exists. Qwik revisions keep working as an emergency rollback until 9c.

#### 9c: Remove the Secret

1. Wait for the rollback window to close (proposed: 30 days after `v5.0.0` with no rollback).
2. Confirm that no revision with traffic, and no revision anyone intends to roll back to, references the secret.
3. Remove `google_secret_manager_secret.supabase_key` and its runtime IAM.
4. Delete or tag-off older revisions that reference it, so nobody rolls back to a revision that can no longer start.

### Phase 10: Remove Qwik

1. Delete `src/`, `adapters/`, `vite.config.ts`, `qwik.env.d.ts`, `tsconfig.json`, `i18n/`, `playwright.config.ts`, `e2e/mock-supabase.ts`, the open-tab spec and its proxy, the Qwik CI jobs, and the Qwik, Vite, Vitest, Lenis and Supabase JS dependencies and overrides. Move `e2e/go/playwright.config.ts` to the root.
2. Keep the `/build/*.js` shim, the service-worker kill switch and the `/assets/` redirects for at least 90 days after `v5.0.0`, then decide whether to drop them.
3. Replace the husky `pre-push` (`bun run verify`) with `make ci`. Narrow Biome to the remaining JS/CSS/JSON/Markdown or replace it, without adding ESLint or Prettier.
4. Rewrite `AGENTS.md` in the Moviestracker shape: Datastar first, DaisyUI first, no inline styles, work loop, TDD at public seams, commands, progressive disclosure.
   - Keep this repo's deployment and secret invariants.
   - Replace the Supabase rules with Firestore ones: runtime read-only IAM, no database keys, content edits in the console, data model in `docs/CONTENT.md`.
   - Add one rule: template changes take effect only after deploy and promotion.
5. Update `.github/*.md` guides, `README.md`, `REVIEW.md`, `.github/copilot-instructions.md`, `.codex/`, and replace `.agents/skills/qwik-aesthetic-core`.
6. Rename the GitHub repository to `aestheticlab` in three steps. The WIF provider conditions and principal bindings (`infra/main.tf:34-37`, `:219`) match `var.github_repository`, and the infrastructure workflow itself authenticates through them, so a direct rename would lock it out.
   1. Apply conditions and bindings that accept both the old and the new name.
   2. Rename the repository.
   3. Apply with only the new name.

## Phase-Wise Gating

- Phase 0 (met): baseline numbers, instance counts and the Firestore database list recorded here; targets and read model finalized.
- Phase 1: OpenTofu format and validate pass, the saved plan shows only probe fields changing, and the staging smoke test passes. The promoted staging revision shows the new probe, and staging `startup_latencies` drops well below the 5.1 s baseline.
- Phase 2: `make ci` passes; the container serves `/healthz` locally; local process-start-to-first-200 is measured; the Qwik CI jobs are unchanged and green.
- Phase 3: `assets-check` shows no diff; a test checks that every manifest entry exists on disk with correct dimensions.
- Phase 4: the translation-completeness test passes for all locales; locale routing tests cover `/`, each locale root, and unknown paths.
- Phase 5:
  - IaC plans reviewed for both environments, with no change to `(default)` or `aestheticlab-income`.
  - The runtime service accounts cannot read either neighboring database (checked with a denied read).
  - Document counts per collection match the Supabase row counts (1 / 8 / 77 / 4 at time of writing).
  - Emulator tests cover the cold-path and in-request refresh cases.
  - `go test -race` passes, and both e2e configurations are green.
- Phase 6: shared Playwright specs pass under both configs; screenshot differences reviewed; no CSP violations in the browser console on any page.
- Phase 7: redirect, service-worker and open-tab specs pass; the revised skew gate is reviewed.
- Phase 8: staging smoke test passes; staging cold starts meet the targets; the Go e2e job is primary in CI.
- Phase 9a:
  - The `v5.0.0` canary passes the revised gate.
  - After the apply and the `v5.0.1` promotion, the serving revision shows min instances 0.
  - `instance_count` reaches 0.
  - Firestore reads stay within the model, and no alert regresses for a week.
- Phase 9b: the promoted revision has no Supabase env or secret reference; cold starts still succeed.
- Phase 9c: no revision with traffic or marked for rollback references the secret before it is removed; cold starts succeed afterwards.
- Phase 10: `make ci`, Markdown lint and the e2e suite pass with Qwik removed; a deploy succeeds after the repository rename.

## Commit Strategy

Not requested yet. When it is: one feature branch and PR to `staging` per phase (Phase 1 ships first, on its own), each commit with the `Co-Authored-By` footer from `AGENTS.md`.

## Verification

- Go: `make ci` (templ and assets checks, `go vet`, staticcheck, golangci-lint, `go test -race ./...` with the Firestore emulator, govulncheck, gosec, build).
- E2E: `bunx playwright test` (Qwik, until Phase 10) and `bunx playwright test -c e2e/go/playwright.config.ts` (Go).
- While Qwik remains: `bunx --bun biome ci .` and `bun run verify`.
- IaC: `tofu -chdir=infra fmt -check -recursive`, `tofu -chdir=infra init -backend=false -input=false`, `tofu -chdir=infra validate`, and a reviewed saved plan for each environment.
- Docs: `markdownlint --disable MD013 -- <changed-files>`.
- Runtime, after each promotion: the serving revision's config (`gcloud run revisions describe`), Cloud Run `container/startup_latencies`, `container/instance_count`, request latency and 5xx metrics, and Firestore read counts.

## Open Questions

1. Who edits content after the move, and through which IaC-managed identity (a Google group, for example) do they get Firestore write access in the console?
2. Is 30 days the right Qwik rollback window before the Supabase secret is removed (Phase 9c)?
