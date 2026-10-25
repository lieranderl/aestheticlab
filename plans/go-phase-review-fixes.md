# Go Migration Review Fixes

## Goal

Resolve the five findings from the completed-phase code review without changing
the deployment cutover or the remaining migration phases.

## Scope and Design

- Preserve the Go server, templ, five locales, embedded assets and existing Qwik CI.
- Use the standard library for HTTP negotiation and Go syntax inspection.
- Compile templ fixtures in memory with the pinned templ parser and generator,
  then inspect their Go syntax; do not parse templ expressions with regular expressions.
- Add no dependencies and change no content or public translation keys.
- Work on `codex/go-phase-review-fixes`. The user requested a commit, PR and
  squash merge to `staging`; monitor the resulting staging deployment.

## Tasks and Gates

1. **Panic recovery** (`internal/server/logging.go`, `logging_test.go`).
   - [x] Reproduce a flushed response followed by a panic through a real HTTP server.
   - [x] Abort the response with `http.ErrAbortHandler` after logging the panic;
     preserve the existing pre-response 500 behavior.
   - [x] Verify both panic paths and the server package with race detection.
2. **Encoding negotiation** (`internal/server/static.go`, `static_test.go`).
   - [x] Reproduce quality ordering, wildcard acceptance, identity rejection and
     no acceptable representation at the HTTP handler seam.
   - [x] Select an available representation by quality; keep Brotli as a tie
     preference, respect explicit exclusions, and answer 406 without caching
     when none is acceptable. Cover multiple header lines and missing siblings.
   - [x] Verify the server package with race detection.
3. **Repeated placeholders** (`cmd/i18n-convert/main.go`, `main_test.go`).
   - [x] Reproduce repeated single arguments, including translation-only repeats.
   - [x] Reuse an indexed argument when a placeholder repeats without changing
     the existing single-occurrence keys or generated tables.
   - [x] Verify the converter and translation packages.
4. **Translation coverage** (`internal/views/i18n_test.go`).
   - [x] Reproduce missed multiline calls, arbitrary context expressions,
     contextual calls and nested templates using temporary source fixtures.
   - [x] Walk the source tree recursively, convert templ to Go in memory, and
     inspect call arguments with the Go AST. Ignore comments, tests and
     generated sources; fail on unsupported dynamic message expressions.
   - [x] Verify scanner fixtures, locale completeness and the views package.
5. **Go container CI** (`.github/workflows/quality.yml`, smoke script).
   - [x] Add a separate job building `Dockerfile.web` for linux/amd64, with its
     own build cache scope and the existing pinned build and scan actions.
   - [x] Smoke-test the non-root image with health endpoints, root redirect,
     compressed CSS/JS, fonts, images and ranges. Preserve session affinity.
   - [x] Check workflow and shell syntax; build and run locally if supported.

## Final Verification

- [x] `make ci` (including generation drift checks and race/security gates).
- [x] `bun run verify` and `bunx --bun biome ci .` for the CI change.
- [x] Markdown lint on this plan; workflow and shell checks.
- [x] Review the complete diff and record validation limits.

## Commit Strategy

One attributed fix commit on `codex/go-phase-review-fixes`, followed by a PR to
`staging`. Wait for every PR check before squash merging and deleting the remote
feature branch. Monitor the automatic staging deployment. No production release
or infrastructure changes are included.

## Verification Record

- All four regression groups reproduced their original failures before fixes.
- `make ci`: passed, including race tests, generation checks, static analysis,
  govulncheck, gosec and the binary build.
- `bun run verify`: passed (121 tests and both production builds).
  Biome reports the pre-existing `innerText` warning in the pricelist e2e spec.
- `bunx --bun biome ci .`, actionlint, shellcheck, Bash syntax and Markdown lint:
  passed. No application dependencies or generated tables changed.
- `Dockerfile.web`: built for linux/arm64 and linux/amd64. Both image variants
  retain user `65532:65532`. The arm64 container passed the smoke script:
  probes, root redirect, all 228 manifest assets, gzip text and video ranges.
- The new GitHub job will run Trivy on the amd64 image; that scan and the hosted
  job were not run locally. The temporary local container was stopped.
- Independent review found one additional cache edge: identity-only assets also
  need `Vary: Accept-Encoding` on successful responses. Its HTTP regression
  reproduced the missing header; the fix sets Vary on all static representations.
  No other findings were reported.
