# Agent Instructions

## Scope

- Canonical policy; nested `AGENTS.md` files override it for their subtree. Preserve user changes and requested scope.
- Aesthetic Lab is a Qwik City/Qwik Speak site with Supabase SSR data, DaisyUI 5, Tailwind CSS 4, Bun, OpenTofu, and Cloud Run.
- Follow `plans/README.md` for multi-step work. Review-only tasks use `REVIEW.md`; `code_review.md` is a compatibility pointer.

## Commands

| Task                                       | Command                                                                                                           |
| ------------------------------------------ | ----------------------------------------------------------------------------------------------------------------- |
| Install local deps                         | `bun install`                                                                                                     |
| Install CI deps                            | `bun ci`                                                                                                          |
| Local dev                                  | `bun run dev`                                                                                                     |
| Format/lint fix                            | `bun run biome`                                                                                                   |
| Format/lint check                          | `bunx --bun biome ci .`                                                                                           |
| Typecheck                                  | `bun run build.types`                                                                                             |
| Unit tests                                 | `bun run test`                                                                                                    |
| Unit coverage                              | `bun run test.coverage`                                                                                           |
| E2E tests                                  | `bun run test.e2e`                                                                                                |
| Production build                           | `bun run build`                                                                                                   |
| Main local verification                    | `bun run verify`                                                                                                  |
| Go site gate (rewrite)                     | `make ci`                                                                                                         |
| Go site dev (Air)                          | `make go-dev`                                                                                                     |
| Go site assets (CSS, fonts, icons, images) | `make assets` (`bun run assets`), then commit `internal/assets/manifest_gen.go` and `internal/views/icons_gen.go` |
| Go site texts (from i18n/*/app.json)       | `make i18n`, then commit `internal/i18n/messages_gen.go`                                                          |
| Translation extraction                     | `bun run qwik-speak-extract`                                                                                      |

## File-Scoped Commands

| Task                   | Command                                           |
| ---------------------- | ------------------------------------------------- |
| Check/fix source file  | `bunx --bun biome check --write path/to/file`     |
| Check source file only | `bunx --bun biome check path/to/file`             |
| Unit test file         | `bunx vitest run path/to/file.test.ts`            |
| E2E spec               | `bunx playwright test path/to/file.spec.ts`       |
| Markdown               | `markdownlint --disable MD013 -- path/to/file.md` |

## Verification Matrix

| Change                                   | Required verification                                                      |
| ---------------------------------------- | -------------------------------------------------------------------------- |
| Docs only                                | `markdownlint --disable MD013 -- <changed-files>`                          |
| App, dependencies, build, deployment     | `bun run verify`                                                           |
| Go site (`cmd/`, `internal/`, `static/`) | `make ci`                                                                  |
| User-facing flow                         | Relevant Playwright spec; manual browser check for styling                 |
| Translation keys                         | Extraction plus affected locale assets, then `make i18n` for the Go tables |
| IaC                                      | Format, validate, and review plan for each affected environment            |

## Repository Boundaries

- `src/routes/` owns Qwik City routes, `routeLoader$`, and `DocumentHead`.
- `src/components/sections/` owns page sections; `src/components/ui/` owns reusable UI primitives with no data fetching.
- `src/shared/` owns Supabase, runtime config, security headers, logging, locale, and service helpers.
- `i18n/` contains Qwik Speak locale assets; update all five locales through extraction when keys change.
- `infra/` owns OpenTofu-managed GCP resources; `.github/workflows/` owns delivery automation.
- `cmd/`, `internal/`, `static/`, `Dockerfile.web` and `.agents/skills/` (Go, Datastar, DaisyUI, TDD) belong to the Go + Datastar rewrite in `plans/go-datastar-scale-to-zero.md`, which replaces the Qwik site; write Go test-first at public seams with `net/http/httptest`.
- `plans/` holds durable execution plans; `docs/prompt-pack.md` holds reusable prompts, not policy.

## Key Conventions

- Use Qwik APIs only: `component$`, signals/stores, `$()` handlers; no React APIs.
- Keep `routeLoader$` in route files. Shared data and locale mapping belong in `src/routes/[...lang]/layout.tsx`; use the per-request Supabase client and fail soft.
- Use `~/*` imports, typed props, existing `src/components/ui/*` primitives, DaisyUI semantic tokens, and `custom-container`; do not add `tailwind.config.js`.
- Use `inlineTranslate()` and `key@@Default English Text`; synchronize all five locales with `bun run qwik-speak-extract`.
- Route files own `DocumentHead`; preserve Consent Mode v2, accessibility, meaningful image `alt`, and stable image sizing.
- Byte-identical optimized images must share one canonical import; alias legacy gallery keys to it. Deployment smoke tests must verify every rendered optimized image URL.
- Canary smoke tests must preserve the session-affinity cookie across page and asset probes; never split one page's checks across revisions intentionally.
- Biome owns formatting. Do not add ESLint or Prettier.

## Dependencies, Env, and Generated Files

- Use the Bun version pinned in `package.json`; change dependencies and `bun.lock` only when required by the task.
- Required runtime env vars: `SUPABASE_URL` and `SUPABASE_KEY`. Keep values only in `.env`, GitHub/Cloud Run secrets, or Secret Manager.
- Never commit `.env`, secret values, service-role keys, `sb_secret_*` keys, or sensitive logs/plans.
- Do not edit or commit generated/build output: `dist/`, `server/`, `node_modules/`, `tmp/`, reports, coverage, logs, or OpenTofu state/plan files.
- No Supabase migration workflow exists in this repo; do not invent one. Document any required schema change and keep application runtime keys publishable/anon only.

## DevOps and IaC

- GitHub Actions is the delivery system; authenticate to GCP with OIDC/WIF only.
- Build, scan, attest, and publish once to Artifact Registry; deploy immutable digests and promote the same digest to production.
- Production promotion is triggered by a published release, environment-protected, smoke-tested before traffic migration, and reversible to a prior Cloud Run revision.
- Manage Cloud Run, dedicated least-privilege service accounts, secret bindings, WIF, monitoring, and alerts in `infra/`; after bootstrap, apply only a saved plan through the protected `infrastructure` workflow and never create console-only drift.
- Never expose secrets in source, workflow inputs, image layers, plans, logs, or CLI arguments. Scope Secret Manager access to individual environment secrets; application runtimes may use only Supabase publishable/anon keys, never `service_role` or `sb_secret_*`.
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
