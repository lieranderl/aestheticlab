# Agent Instructions

## Scope

- Canonical policy; nested `AGENTS.md` files override it for their subtree. Preserve user changes and requested scope.
- Aesthetic Lab is a Go + templ + Datastar site with Firestore content, DaisyUI 5 and Tailwind CSS 4 (built with Bun), OpenTofu, and Cloud Run.
- Follow `plans/README.md` for multi-step work. Review-only tasks use `REVIEW.md`; `code_review.md` is a compatibility pointer.

## Commands

| Task                                   | Command                                                                                                           |
| -------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| Install local deps                     | `bun install`                                                                                                     |
| Install CI deps                        | `bun ci`                                                                                                          |
| Local dev (Air)                        | `make dev`                                                                                                        |
| Format/lint fix (JS, CSS, JSON, MD)    | `bun run biome`                                                                                                   |
| Format/lint check                      | `bunx --bun biome ci .`                                                                                           |
| Main local verification                | `make ci` (also the pre-push hook)                                                                                |
| Browser tests                          | `make test-e2e` (Chromium, Firefox and WebKit; embedded snapshot, local HTTPS)                                    |
| Build the binary                       | `make build` (`bin/web`)                                                                                          |
| Assets (CSS, fonts, icons, images)     | `make assets` (`bun run assets`), then commit `internal/assets/manifest_gen.go` and `internal/views/icons_gen.go` |
| Texts (`internal/i18n/locales/*.json`) | `make i18n`, then commit `internal/i18n/messages_gen.go`                                                          |
| Content tests (Firestore emulator)     | `make firestore`, then `make test-content`                                                                        |
| Content snapshot (from production)     | `make snapshot`, then commit `internal/content/snapshot.json`; see `docs/CONTENT.md`                              |

## File-Scoped Commands

| Task                   | Command                                           |
| ---------------------- | ------------------------------------------------- |
| Check/fix source file  | `bunx --bun biome check --write path/to/file`     |
| Check source file only | `bunx --bun biome check path/to/file`             |
| Go package tests       | `go test -race ./internal/<package>/`             |
| Bun test file          | `bun test path/to/file.test.mjs`                  |
| E2E spec               | `bunx playwright test path/to/file.spec.ts`       |
| Markdown               | `markdownlint --disable MD013 -- path/to/file.md` |

## Verification Matrix

| Change                                | Required verification                                           |
| ------------------------------------- | --------------------------------------------------------------- |
| Docs only                             | `markdownlint --disable MD013 -- <changed-files>`               |
| Site, dependencies, build, deployment | `make ci`                                                       |
| User-facing flow                      | Relevant Playwright spec; manual browser check for styling      |
| Texts                                 | All five locale files, then `make i18n`                         |
| IaC                                   | Format, validate, and review plan for each affected environment |

## Repository Boundaries

- `cmd/web` is the site; `cmd/content` imports and exports Firestore content; `cmd/i18n-convert` writes the Go text tables.
- `internal/server` owns routing, locales, headers, compression, static files and the old Qwik URLs (`compat.go`); `internal/views` owns templ pages and sections; `internal/content` owns the Firestore reader, cache and snapshot; `internal/i18n` owns texts and their locale files; `internal/assets` owns the asset manifest.
- `frontend/` owns the asset sources and build: `app.css`, `site.js`, vendored Datastar, `media/` and `public/`. The built files go to `static/dist/`, which the binary embeds.
- `e2e/` holds the browser specs; `scripts/` the deploy smoke tests and the link-preview renderer.
- `infra/` owns OpenTofu-managed GCP resources; `.github/workflows/` owns delivery automation; `.agents/skills/` holds the Go, Datastar, DaisyUI and TDD skills.
- `plans/` holds durable execution plans; `docs/CONTENT.md` documents the Firestore content; `docs/prompt-pack.md` holds reusable prompts, not policy.

## Key Conventions

- Write Go test-first at public seams with `net/http/httptest`.
- Use Datastar for interaction and DaisyUI semantic components and tokens for styling; no inline styles, no `tailwind.config.js`.
- Texts are looked up by their English (`internal/i18n`); add a text to all five locale files, then `make i18n`.
- Preserve Consent Mode v2, accessibility, meaningful image `alt`, and stable image sizing.
- Byte-identical images share one canonical asset. Deployment smoke tests must verify every rendered image URL.
- Canary smoke tests must preserve the session-affinity cookie across page and asset probes; never split one page's checks across revisions intentionally.
- Biome owns JS, CSS, JSON and Markdown formatting; `gofmt` and golangci-lint own Go. Do not add ESLint or Prettier.

## Dependencies, Env, and Generated Files

- Use the Bun version pinned in `package.json` and the Go version in `go.mod`; change dependencies, `bun.lock` and `go.sum` only when required by the task.
- Runtime env vars: `FIRESTORE_DATABASE` and `GOOGLE_CLOUD_PROJECT`, set by `infra/`. Without a database the site serves its embedded content snapshot.
- The runtime reads Firestore with read-only, database-conditioned IAM and holds no database keys. Content is edited in the Firestore console; the data model is in `docs/CONTENT.md`.
- Never commit `.env`, secret values or sensitive logs/plans.
- Do not edit or commit build output: `bin/`, `static/dist/`, `node_modules/`, `tmp/`, reports, coverage, logs, or OpenTofu state/plan files. Do commit the generated Go files the checks compare (`*_templ.go`, `manifest_gen.go`, `icons_gen.go`, `messages_gen.go`).

## DevOps and IaC

- GitHub Actions is the delivery system; authenticate to GCP with OIDC/WIF only.
- Build, scan, attest, and publish once to Artifact Registry; deploy immutable digests and promote the same digest to production.
- Production promotion is triggered by a published release, environment-protected, smoke-tested before traffic migration, and reversible to a prior Cloud Run revision.
- Manage Cloud Run, dedicated least-privilege service accounts, Firestore databases and their IAM, WIF, monitoring, and alerts in `infra/`; after bootstrap, apply only a saved plan through the protected `infrastructure` workflow and never create console-only drift.
- Never expose secrets in source, workflow inputs, image layers, plans, logs, or CLI arguments. Scope Secret Manager access to individual secrets; the site's runtime needs none.
- Keep notification recipient addresses outside OpenTofu plans and state; bootstrap channels securely in GCP and reference only their non-sensitive resource names from IaC.

## Instruction Maintenance

- Update this file or the nearest scoped guide when commands, stack, deployment flow, or hard invariants change.
- If the same correction appears in review twice, add a short enforceable rule here or in the relevant `.github/*.md` guide.
- Keep `AGENTS.md` concise; move detailed examples to `.github/`, `plans/README.md`, `REVIEW.md`, or nested `AGENTS.md`.

## Git and Commit Attribution

- Use a feature branch. Validate before PR; use `gh` and target `staging`: `gh pr create --fill --base staging`.
- Do not commit, push, create a PR, deploy, or alter cloud resources unless explicitly requested.
- AI commits MUST include `Co-Authored-By: <agent model> <agent attribution email>` using the acting agent's identity.

## Deployments

### Deploy to Staging

To deploy changes to the staging environment, do not push directly to `staging`. Follow these steps:

1. Create and switch to a feature branch (e.g., `feat/...` or `fix/...`).
2. Commit your changes including the required `Co-authored-by` footer.
3. Push your feature branch to the remote repository.
4. Create a Pull Request targeting the `staging` branch: `gh pr create --fill --base staging`.
5. Monitor and wait for all PR checks to pass: `gh pr checks <pr-number>`.
6. Squash merge the pull request and delete the remote branch: `gh pr merge <pr-number> --squash --delete-branch`.
7. A merge/push to `staging` automatically triggers the **Build and promote to Cloud Run** GitHub Actions workflow. Monitor the deployment progress with `gh run list` and `gh run view <run-id>`.

### Deploy to Production

Production deployments are triggered by publishing a new GitHub release tag that matches the `version` field in `package.json`.

1. Verify the current project version in `package.json` matches your intended release tag.
2. Publish a new GitHub Release targeting the `staging` branch:
   `gh release create vX.Y.Z --target staging --title "vX.Y.Z" --notes "Release notes"`
3. The release trigger automatically starts the production deployment workflow. Monitor its status to ensure the canary split, smoke testing, and promotion complete successfully.

## References

- Review/deployment/data: `REVIEW.md`, `code_review.md`, `.github/DEPLOYMENT.md`, `.github/DATA_LOADING.md`.
- UI/i18n: `.github/COMPONENT_GUIDE.md`, `.github/DAISYUI_PATTERNS.md`, `.github/I18N_GUIDE.md`.
- Human setup/entrypoint: `README.md`, `.codex/README.md`.
