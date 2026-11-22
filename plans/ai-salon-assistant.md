# AI Salon Assistant (Chat with Hand-off to Timely)

Status: **proposal, 2026-11-22**. Owner review needed before any code.

## Goal

A chat on the site answers visitors' questions about Aesthetic Lab: treatments, prices, durations, technicians, opening hours, location, aftercare and policies. It suggests free times from the staff calendars and hands the visitor off to Timely's booking widget with the treatment and technician already chosen. Timely stays the only place where an appointment is made.

## Decisions

| Topic | Decision | Reason |
| --- | --- | --- |
| Booking | **Hand-off to Timely**: the chat shows a Book button that opens the existing booking dialog | Timely has no usable API. An event the site writes to Google Calendar reaches Timely only as a greyed busy block, not an appointment. The widget already takes `product` and `staff` (`internal/views/booking.go`), and Firestore service and staff IDs are Timely's IDs, so the hand-off pre-selects both. |
| Availability source | Google Calendar `freebusy.query` on the four staff calendars that Timely two-way syncs | Same calendars the income app reads (`aestheticlab-income-app/internal/bookings`). Free/busy needs no event text, so no client data reaches the site. |
| Calendar access | Share each staff calendar with the site's runtime service accounts at **"See only free/busy (hide details)"**; scope `calendar.freebusy`; Application Default Credentials | No keys, least privilege. The income app keeps its own account and its "See all event details" share. |
| Working hours and skills | New fields on existing Firestore documents: `staff.hours`, `staff.booking_step`, `staff.days_off`, and the staff who do each **service** (`services.staff_ids`) | The calendars show busy time, not rosters. Timely assigns staff per service, not per group (laser services in one group have different staff), so a per-group list would offer wrong people. Edited in the console like the rest of the content. |
| Model access | Gemini through **Vertex AI** at the **`eu`** multi-region endpoint, with the runtime service account and the official Go library `google.golang.org/genai` (Vertex backend, Application Default Credentials) | Google is the only data processor, data stays in the EU, and the runtime holds no secrets. Google's own models use a shared quota, so no quota request is needed. |
| Model | **Gemini 3.1 Flash-Lite** (`gemini-3.1-flash-lite`), chosen by the owner (2026-11-22) | Claude Haiku 5.5 was the first choice, but Google automatically denied every EU quota request for it in `nail-lab-449417` (2026-11-22). Flash-Lite costs $0.275 / $1.65 per million input/output tokens in the EU ($0.0275 cached), about $3 a month at 500 chats, and supports function calling and implicit caching. The model stays one setting, so Haiku can return once quota exists. |
| Agent loop | A short function-calling loop in the Go handler, at most 4 tool rounds per turn | Two read-only tools and short chats; nothing needs a hosted agent. |
| Conversation state | Stateless server. The browser keeps the visible transcript (text only) and sends it back; each turn is one new request whose user message carries the transcript as quoted data | Scale to zero and several instances rule out server memory, and the runtime can't write Firestore. Sending the transcript as quoted data, not as model turns, means a forged "assistant" line is just user-supplied text. |
| Topic scope | System prompt limits the bot to the salon; facts come only from the content in the prompt; no web or general tools; medical questions go to a consultation | See "Staying on topic". |
| Transport and UI | `POST /{locale}/chat` answers with Datastar SSE, streaming the reply into a DaisyUI chat panel | Same pattern as `ServicesFragment` in `internal/server/pages.go`. Same origin, so the CSP needs no change. |
| Personal data | The chat asks for none and logs no message text; usage is logged as token counts only | Belgium, GDPR. The privacy policy names the model provider as a processor. |

## Non-Goals

- Creating, moving or cancelling appointments, and holding slots in Google Calendar.
- Collecting names, phone numbers or e-mail addresses in the chat (booking requests are a possible later plan).
- Reading event details, client names or Timely prices from the calendars.
- Changing the income app, the Telegram bot or their service accounts.

## Assumptions

- Firestore `services` IDs are Timely product IDs and `staff` IDs are Timely staff IDs, as today's Book buttons rely on. Phase 0 checks this against the live widget.
- Timely writes every booking and "Busy Time" block to the staff calendars as busy, under 24 hours, with a lag of up to about 15 minutes. The chat says a time "looks free".
- The four staff calendars are owned by the salon Google account, which can share them.
- Traffic is low: hundreds of conversations a month, not thousands.

## Staying on Topic

1. **System prompt:** the assistant of Aesthetic Lab in Leuven; answers only about the salon; for anything else, one polite sentence and a salon suggestion; replies in the page's locale or the visitor's language.
2. **Grounding:** the prompt carries the locale's treatments (name, description, price, duration, staff), groups, staff (name, role, bio), contact facts and hours (`internal/views/site.go`), and the FAQ texts. Rule: if it isn't there, say so and offer Instagram, e-mail or phone; never guess a price or duration.
3. **Tools:** only `find_free_slots` and `offer_booking`. Both validate their IDs against the content.
4. **Medical boundary:** general treatment information only; contraindications, pregnancy, medication and skin conditions go to a consultation.
5. **Limits:** message length (500 characters), turns per conversation (20), reply length, tool rounds (4), a per-IP rate limit in each instance, and a `CHAT_ENABLED` kill switch. Cloud Run's max instances caps the total.
6. **Eval set:** Phase 6.

## Local Testing First

Everything runs on the laptop before anything is deployed (owner, 2026-11-22):

| Part | Locally |
| --- | --- |
| Content | The Firestore emulator (`make firestore`, Apple `container` on this Mac) loaded with a copy of staging, which has the new `hours`, `booking_step` and `staff_ids` |
| Model | Real Gemini 3.1 Flash-Lite at `eu`, with your `gcloud` login (tested: HTTPS works from this network; Firestore's gRPC doesn't, hence the emulator) |
| Model, for tests | A fake model (`CHAT_MODEL=fake`): scripted answers and tool calls, no cost, used by the Go tests and Playwright |
| Calendars | A made-up dev calendar (`AVAILABILITY_DEV=1`, like the income app's `INCOME_DEV_BOOKINGS`) until the calendars are shared; then the real ones, read as the staging service account (`--impersonate-service-account`) |
| Site | `make dev` at `http://localhost:3000`, plus `make test-e2e` |

Only after the local checks pass does anything reach staging.

## Progress (2026-11-22)

Built on `feat/ai-salon-assistant`, not committed. `make ci` passes.

- `internal/availability`: schedules, free-slot logic, `Finder`, Google `freeBusy` source (a calendar with errors is never free), `DevSource`, `AVAILABILITY_CALENDARS` parsing.
- `internal/assistant`: system prompt from content, FAQ and facts (byte-identical per locale for caching), `find_free_slots` and `offer_booking` tools, a loop of at most 4 rounds, `Gemini` (Vertex, `eu`) and `Fake` models.
- `internal/server/chat.go`: `POST /{locale}/chat` over Datastar SSE, cross-origin protection, 64 KB body, 500-character messages, last 20 turns, per-visitor and global rate limits, fixed fallback replies, logs without message text.
- `internal/views/chat.templ`: launcher and panel (DaisyUI), Book buttons through `BookButton`; 13 texts in all five locales.
- `cmd/web`: `CHAT_ENABLED`, `CHAT_MODEL` (default `gemini-3.1-flash-lite`, or `fake`), `CHAT_LOCATION` (default `eu`), `AVAILABILITY_CALENDARS`, `AVAILABILITY_DEV`.

Local run (launch config `go-web-chat`, port 3013: emulator with the staging copy, real Gemini, dev calendar): answers prices and staff correctly, finds times, its Book button opens Timely on the time picker with treatment and person chosen, refuses off-topic and override prompts, answers Dutch in Dutch, sends medical questions to a consultation.

All four staff calendars are shared with the runtime accounts; their IDs are kept out of git (local `.env.chat.local`, later `infra/terraform.tfvars`). The `CALENDAR_IDS` secret lists them by name.

Before staging (2026-11-22, done): `e2e/chat.spec.ts` with the fake model (the widget stays hidden without JavaScript); privacy-policy lines shown only where the chat runs; `infra/` adds the Vertex AI API, `roles/aiplatform.user` for `chat_environments` (default staging), the chat env vars and the sensitive `availability_calendars` variable from the `AVAILABILITY_CALENDARS` GitHub secret; `AGENTS.md` and `infra/README.md` updated.

Assistant notes (2026-11-22): the owner's facts live in the `assistant_notes` collection (English; `topic`, `text`, `active`), with optional `assistant_note` fields on services and service groups; the bot also reads the care notes page (`/notice/`). Staging has `cancellation`, `payment` and `late-arrival`. Days off come through the calendars (owner: Timely writes them there), so `days_off` stays empty. Everything stays in staging until it is tested there.

Spending caps (2026-11-22): `internal/usage` prices every reply's tokens (Vertex AI prices + 25%) into a `chat-usage` Firestore ledger shared by all environments, and the chat stops at €9 a month or €0.50 a day; an unreadable ledger or an unpriced model keeps it off. Per visitor: 6 at once, one every 10 s, 40 a day; per instance: 30 a minute. A €10 Google Cloud budget on Vertex AI and the Gemini API (AestheticLab billing account) alerts at 50/90/100% and on forecast, and two alert policies cover cap events and volume spikes. Measured locally with real Gemini: about €0.001 a reply once the prompt is cached (€0.003 for the first).

Test set (2026-11-22): 62 cases (66 messages) in all five locales against the local server with real Gemini 3.1 Flash-Lite, staging content and the dev calendar; 95 replies cost €0.32 (about €0.003 each, margin included). 52 passed cleanly: every off-topic (6), override (8) and medical (3) prompt was handled, no price or treatment was made up, men's and women's laser prices stayed apart, and times stayed within each person's hours and the 14-day window. Found and fixed: four booking answers shown twice, one with times written before `find_free_slots` returned (the reply is now the final answer, text written with a `find_free_slots` call is dropped, and a rewrite replaces the first answer); "I have selected 16:00" (the Book button can't choose a time); lash-lift aftercare not in the care notes (care advice only as the care notes or FAQ say); "tap the button below" with no button. The `eu` endpoint answered one 504 and one 429; the Gemini client now retries once. A re-run of the affected cases passed.

Studio and treatment details (2026-11-22): the assistant also gets the site's category descriptions and studio texts, full treatment descriptions (up to 800 characters) and staff bios (600), and is told to be warm without exaggerating. Owner notes in `assistant_notes`: `about-studio` (ground floor, no stairs, coffee, tea or water, Wi-Fi, calm music), `hygiene` (three-stage sterilization), `nail-products` (TPO-free, no brands), `pedicure-technique` (dry, no foot bath), `laser` (diode, test patch; typical sessions and intervals from public sources), `lash-lift`, `brow-lamination`, `face-waxing` (typical durations and aftercare from public sources). Where a treatment's own description gives a figure (lash lift: up to 4–5 weeks), the note leaves it out. Owner corrections on staging: the Korean lash lift curls and sets the lashes (it doesn't strengthen them) and lasts 4 to 8 weeks, in its description (all five locales) and note; brow lamination lasts up to 3 months.

Cut replies (2026-11-22): a Russian staging answer stopped mid-word at 1,020 of 1,024 output tokens (Gemini's thinking counts). The limit is now 2,048, and a reply that still hits it ends at its last whole sentence.

Test set in the repo (2026-11-22): `scripts/chat-test-set.json` holds the 71 cases (75 messages) with what a good answer says; `bun run chat-test-set` sends them to a local server with a real model and saves the transcripts in `tmp/chat-test-set/` for grading. Re-run it after big content changes or a model change (about €0.25). The prompt now says an owner note wins over a treatment's description when they disagree.

Still to do: ~~create the `AVAILABILITY_CALENDARS` repository secret~~ (done); ~~review and apply the infrastructure plan; PR to `staging`; check real calendars and Gemini on staging; the 60-question test set~~ (done); check free times against the Timely widget on staging; update the privacy policy's "Last updated" date when the chat reaches production; seed production Firestore.

## Phases

### Phase 0: Checks and Owner Input

- Confirm on the live widget that `?product=<Firestore service id>&staff=<Firestore staff id>` opens with both selected, for at least one service per group.
- Vertex AI API is on in `nail-lab-449417` (checked 2026-11-22). Confirm one Gemini 3.1 Flash-Lite call succeeds at the `eu` endpoint.
- Owner provides each person's weekly hours and booking step from Timely's "Normal working hours". Timely's public booking page lists which staff do each service.
- Owner approves the privacy-policy wording and a monthly spend ceiling.

Gate: all four answered; this plan updated where an assumption broke.

#### Found So Far (2026-11-22)

**Hand-off verified.** The site's widget URL with `product=4847871&staff=642297` (Pedicure, Zara) opens Timely directly on the time picker with both selected: Timely rewrites it to `productId` and `staffId`. On 2026-11-22 it offered Monday 23 November 10:00–10:45 and 15:00–15:30, a reference for checking the free-slot logic.

**IDs match.** Timely's public booking page (location `288067`) lists 93 online services with the staff who do each. 74 of the 75 active Firestore services are among them under the same ID; the other, "Coating removal (our salon)" (`4884910`), isn't bookable online. The 19 Timely services missing from Firestore include the junior technician's own services. Staff IDs match too: Rubina `642296`, Zara `642297`, Julia `672362`, Lera `693357`.

**Model.** Claude Haiku 5.5 is offered at `eu` in Model Garden, but its EU quotas are 0 and the increase requests were denied automatically (2026-11-22). The chat uses Gemini 3.1 Flash-Lite at `eu` instead: a test call succeeded on 2026-11-22 at `https://aiplatform.eu.rep.googleapis.com/v1/projects/nail-lab-449417/locations/eu/publishers/google/models/gemini-3.1-flash-lite:generateContent` (on-demand traffic, no quota request). Gemini 3.5 Flash-Lite also answers there and is the first step up; the owner kept 3.1 Flash-Lite for its lower price.

**Who does what** (services per staff set on Timely's page):

| Staff | Services |
| --- | --- |
| Rubina, Zara, Julia | 31 |
| Rubina, Zara, Lera | 23 |
| Julia only | 14 |
| Lera only | 14 |
| Zara, Julia | 9 |
| Rubina only | 2 |

**Normal working hours from Timely** (no breaks; "Reset the online booking time after a break" off):

| Staff | Mon–Fri | Sat | Sun | Booking step |
| --- | --- | --- | --- | --- |
| Rubina (`642296`) | 12:00–19:00 | 11:00–15:00 | off | 15 min |
| Zara (`642297`) | 10:00–18:30 | off | off | 15 min |
| Julia (`672362`) | 11:00–18:00 (Thu 11:00–16:30) | off | off | 15 min |
| Lera (`693357`) | 14:00–19:00 (Fri 12:00–19:00) | 11:00–15:00 | off | 15 min |

Still to check:

- The site's published hours ("Monday – Saturday · 10:00 – 18:00") stay as they are; this plan doesn't change them (owner, 2026-11-22). The chat finds times from the staff hours above, which differ (weekdays until 19:00, Saturday until 15:00).
- Whether Timely lets a service end exactly at the end of a shift, and how it handles days off and holidays (rostered changes beyond normal hours). If Timely writes time off to the calendars as "Busy Time", Firestore needs no exceptions for it.

### Phase 1: Content for Hours and Skills

- Add `services.staff_ids` and the `staff` fields `hours`, `booking_step` and `days_off` to the reader in `internal/content`, test-first. Fields on the existing documents, not a new collection, so Firestore reads, import/export and the snapshot need no change. Malformed hours give no schedule (no times offered), never a partly read one. **Done 2026-11-22** (`internal/availability` holds the schedule types and the free-slot logic). **Staging seeded 2026-11-22** through the Firestore REST API with field masks (only `hours`, `booking_step` and `staff_ids` written; every other field checked unchanged): 4 staff, 74 services; the two inactive services and "Coating removal (our salon)" have no `staff_ids`. Production is not seeded yet.
- Document both in `docs/CONTENT.md`; import them to staging, then production; refresh `internal/content/snapshot.json`.

Gate: `go test -race ./internal/content/`; `make firestore` then `make test-content`.

### Phase 2: Free-Slot Domain

- A pure function: `availability.Schedule.Starts(day, loc, duration, busy, earliest)` returns one person's start times in `Europe/Brussels`, on Timely's grid from the day's first shift. The caller sets `earliest` (now plus lead time).
- Tests: daylight-saving days, bookings overlapping a shift edge, back-to-back bookings, a day off, lead time near now, a treatment longer than every gap, and Timely's offer for Zara on 23 November. **Done 2026-11-22.**

Gate: `go test -race` on the new package, with every case table-driven.

### Phase 3: Calendar Gateway

- `Source` interface, memory fake and Google implementation over `Freebusy.Query` (`calendar.CalendarFreebusyScope`), all calendars in one call, 3 s timeout, `Warm()` at startup. Copy the patterns from the income app; don't share a module.
- **A per-calendar `errors` entry means that calendar is unavailable, never free.** A missing share returns HTTP 200 with an empty busy list.
- `AVAILABILITY_CALENDARS=<calendarId>|<staffId>;…`: parsed and validated at startup; unset turns availability off and the chat says it can't check times.
- `infra/`: a variable for the mapping (kept in the git-ignored `terraform.tfvars`) and the env var on both services. No IAM change: calendar sharing grants the access.
- Owner shares the four calendars, free/busy only, with `aesthetic-stage-runtime` and `aesthetic-prod-runtime`.

Gate: httptest tests for success, a per-calendar error, a timeout and a malformed response; `tofu fmt`, `validate` and a reviewed plan for both environments.

### Phase 4: Model Gateway and Tools

- `internal/assistant`: builds the cached system prompt from content per locale, runs the tool loop, and maps results to events the handler streams. A fake model drives the tests.
- `find_free_slots(service_id, staff_id?, from, to)`: at most 14 days ahead; returns up to 6 slots with staff ID and name.
- `offer_booking(service_id, staff_id?)`: returns nothing to the model; the server renders a Book button for that service and person through `BookButton`. The model never writes a URL.
- Gemini specifics: function declarations for both tools; the system instruction kept byte-identical so implicit caching applies; the lowest thinking level; a small `maxOutputTokens`; `safetySettings` at defaults.
- Handle a blocked or truncated answer (`finishReason` `SAFETY`, `MAX_TOKENS` or an empty reply) with a short fixed reply that offers contact options.
- `infra/`: `roles/aiplatform.user` for both runtime accounts.

Gate: tests for an unknown service ID, slots when availability is off, the tool-round limit, a blocked answer, and a system instruction that is byte-identical across requests (cache hits).

### Phase 5: Chat Endpoint and UI

- `POST /{locale}/chat`: same-origin check, input and transcript limits, rate limit, `CHAT_ENABLED`, SSE streaming; logs only status, token counts and latency.
- UI: a chat button and DaisyUI panel; streamed reply; Book buttons open the existing booking dialog; an "AI assistant, may make mistakes, don't share health details" note; keyboard and screen-reader support.
- New texts in all five locale files, then `make i18n`. Analytics events (`chat_opened`, `chat_booking_offered`) only through the consent-gated tracker.

Gate: handler tests with the fake model; a Playwright spec (`e2e/chat.spec.ts`) on all three browsers covering ask, answer, Book button and dialog; a manual styling check on phone and desktop.

### Phase 6: Evaluation and Model Choice

- About 60 prompts: salon questions in all five locales, availability requests, off-topic requests, medical questions, attempts to override instructions, and requests for a made-up price.
- Grade each answer for staying on topic, factual accuracy against the content, and a correct hand-off. Record the cost per conversation from `usage`.
- Run on Gemini 3.1 Flash-Lite; if it misses the bar, try Gemini 3.5 Flash-Lite, or Claude Haiku 5.5 once it has quota. Every run is billed (well under $1), so the owner approves the run budget first.

Gate: no fabricated prices or treatments; off-topic and override prompts refused; results recorded here. Passed on 3.1 Flash-Lite (Progress, test set).

### Phase 7: Rollout

- Staging behind `CHAT_ENABLED`; owner tries it; then production.
- A Cloud Monitoring alert on chat request volume and a billing budget alert at the agreed ceiling.
- Update the privacy policy, `AGENTS.md` (new env vars, the assistant package) and `docs/CONTENT.md`.

Gate: deploy smoke tests pass; a production chat about one service and one person opens the widget with both selected.

## Verification

- `make ci` for every code phase.
- `go test -race ./internal/<package>/` for content, the slot domain, the calendar gateway and the assistant.
- `bunx playwright test e2e/chat.spec.ts`, then `make test-e2e`.
- `tofu -chdir=infra fmt -check -recursive`, `init -backend=false`, `validate`, and a reviewed plan for staging and production.
- `markdownlint --disable MD013 -- plans/ai-salon-assistant.md docs/CONTENT.md`.

## Commit Strategy

Only when the user asks: one feature branch, one commit per phase, PR to `staging`.

## Open Questions

1. Does the widget pre-select `product` and `staff` together, or does `staff` reset the service? (Phase 0)
2. Should the chat show on every page, or only the home page and pricelist?
3. Keep a conversation's transcript after a page reload (browser storage), or start fresh?
4. Which languages should the bot answer in when the visitor writes in one the site doesn't have?
5. When asked "when are you open?", should the bot quote the site's published hours or the staff hours?
