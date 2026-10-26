# Aesthetic Lab

![Release](https://img.shields.io/github/v/release/lieranderl/qwik-aestheticlab?style=flat&logo=github)
[![Deploy to Cloud Run](https://github.com/lieranderl/qwik-aestheticlab/actions/workflows/deploy.yml/badge.svg?branch=staging)](https://github.com/lieranderl/qwik-aestheticlab/actions/workflows/deploy.yml)
![Qwik](https://img.shields.io/badge/Qwik-%2318B6F6.svg?style=flat&logo=qwik&logoColor=white)
![DaisyUI](https://img.shields.io/badge/daisyUI-5A0EF8?style=flat&logo=daisyui&logoColor=white)
![TailwindCSS](https://img.shields.io/badge/Tailwind_CSS-%2338B2AC.svg?style=flat&logo=tailwind-css&logoColor=white)
![Supabase](https://img.shields.io/badge/Supabase-%233ECF8E.svg?style=flat&logo=supabase&logoColor=white)
![Bun](https://img.shields.io/badge/Bun-%23000000.svg?style=flat&logo=bun&logoColor=white)
![Biome](https://img.shields.io/badge/Biome-%23F9A42F.svg?style=flat&logo=biome&logoColor=white)
![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)

Aesthetic Lab is a multilingual marketing and booking site for a beauty studio in Belgium. It uses Qwik City, Supabase-backed content, DaisyUI/Tailwind styling, and a GitHub Actions delivery pipeline to Google Cloud Run.

## Features

- Localized site for `en-BE`, `nl-BE`, `fr-BE`, `ru-BE`, and `uk-BE`.
- Home and pricelist pages with Supabase-backed services, staff, and contact data.
- GetTimely booking, Google Maps, Google Analytics Consent Mode v2, and cookie consent.
- Health/readiness endpoints for Cloud Run and deployment smoke tests.
- Staging and production deployment through GitHub Actions.

## Stack

| Area           | Technology                          |
| -------------- | ----------------------------------- |
| Runtime        | Bun `1.4.2`                         |
| Framework      | Qwik `1.20`, Qwik City              |
| Language       | TypeScript `7`                      |
| Styling        | Tailwind CSS `4`, DaisyUI `5`       |
| Data           | Supabase SSR `0.12`                 |
| i18n           | Qwik Speak                          |
| Tests          | Vitest with V8 coverage, Playwright |
| Quality        | Biome                               |
| Hosting        | Google Cloud Run                    |
| Infrastructure | OpenTofu                            |

## Local Setup

Install dependencies:

```bash
bun install
```

Create the local environment file:

```bash
cp .env.example .env
```

Set the Supabase runtime values:

```bash
SUPABASE_URL="https://your-project.supabase.co"
SUPABASE_KEY="your-publishable-or-anon-key"
```

Use only a Supabase publishable or anon key. Do not use `service_role` or `sb_secret_*` keys.

Start the dev server:

```bash
bun run dev
```

Open [http://localhost:5173](http://localhost:5173).

## Commands

| Command                      | Description                                                  |
| ---------------------------- | ------------------------------------------------------------ |
| `bun ci`                     | Install dependencies exactly as CI does.                     |
| `bun run dev`                | Start the Qwik City dev server.                              |
| `bun run build`              | Build the production app.                                    |
| `bun run build.types`        | Run TypeScript checks.                                       |
| `bun run test`               | Run Vitest tests.                                            |
| `bun run test.coverage`      | Run Vitest with enforced V8 coverage thresholds.             |
| `bun run test.e2e`           | Run the Playwright specs against the Go site.                |
| `bun run test.e2e.qwik`      | Run the Playwright specs against the Qwik site.              |
| `bun run verify`             | Run Biome, type checks, unit coverage, and production build. |
| `bun run biome`              | Run Biome with fixes.                                        |
| `bunx --bun biome ci .`      | Check formatting and lint without modifying files.           |
| `bun run qwik-speak-extract` | Extract translation keys into locale files.                  |
| `bun run docker.build`       | Build local Docker image `aestheticlab:local`.               |
| `make help`                  | Show Makefile targets.                                       |
| `make go-dev`                | Start the Go site with hot reload on port 3000.              |
| `make test-e2e`              | Run the shared browser specs against the Go binary.          |

The Go browser tests build assets and the binary, then start isolated test
servers on ports 43122 and 43123 using the embedded content snapshot. They
need Go, Bun, OpenSSL and Playwright's Chromium, Firefox and WebKit browsers
(`bunx playwright install chromium firefox webkit`). Local HTTPS keeps the
production CSP intact, including WebKit's insecure-request upgrades. The
certificate is temporary and is removed when the test server exits. Reports
are separate under `playwright-report/go/` and `playwright-report/qwik/`.

## Project Structure

```text
src/
|-- components/      # Sections and reusable UI
|-- constants/       # Navigation and metadata
|-- media/           # Source images and SVGs
|-- routes/          # Qwik City routes and loaders
|-- shared/          # Supabase, runtime config, security, locale helpers
|-- entry.bun.ts     # Bun production server
`-- speak-config.ts  # Locale configuration

i18n/                # Qwik Speak locale files
e2e/                 # Playwright specs
infra/               # OpenTofu infrastructure
scripts/             # Smoke/deployment helper scripts
.github/workflows/   # Quality, deployment, and infrastructure workflows
```

## Development Notes

- Read [AGENTS.md](AGENTS.md) before making changes.
- Use Qwik APIs only: `component$`, `routeLoader$`, signals/stores, and `$()` handlers.
- Keep Supabase reads server-side in route loaders.
- Select explicit Supabase columns and validate/project raw rows before returning loader data.
- Use `inlineTranslate()` with `key@@Default English Text` for user-facing strings.
- Run `bun run qwik-speak-extract` after adding translation keys.
- Use existing DaisyUI/Tailwind patterns and `src/components/ui/*` primitives.

More detailed guides:

- UI: [.github/COMPONENT_GUIDE.md](.github/COMPONENT_GUIDE.md)
- i18n: [.github/I18N_GUIDE.md](.github/I18N_GUIDE.md)
- Data loading: [.github/DATA_LOADING.md](.github/DATA_LOADING.md)
- Deployment: [.github/DEPLOYMENT.md](.github/DEPLOYMENT.md)
- Review process: [REVIEW.md](REVIEW.md)

## Verification

For most changes:

```bash
bun run verify
```

### Test Coverage

`bun run test.coverage` measures the pure/server-side TypeScript modules under
`src/shared/` with Vitest's V8 provider. Generated HTML and JSON reports are
written to the ignored `coverage/` directory.

CI enforces these global minimums:

| Metric     | Minimum | Current baseline |
| ---------- | ------: | ---------------: |
| Statements |     80% |           80.44% |
| Branches   |     75% |           75.87% |
| Functions  |     90% |           90.47% |
| Lines      |     84% |           84.64% |

Playwright coverage is reported separately as passed browser scenarios; it is
not mixed into the unit-coverage percentage. The deterministic suite currently
runs 60 scenarios across Chromium, Firefox, and WebKit locally, while CI runs
the Chromium project.

For docs-only changes:

```bash
markdownlint --disable MD013 -- <changed-files>
```

For focused checks:

| Change                 | Command                                                                                                                       |
| ---------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| Source file            | `bunx --bun biome check --write path/to/file`                                                                                 |
| Source file check only | `bunx --bun biome check path/to/file`                                                                                         |
| Unit test              | `bunx vitest run path/to/file.test.ts`                                                                                        |
| Unit coverage          | `bun run test.coverage`                                                                                                       |
| E2E spec               | `bunx playwright test path/to/file.spec.ts`                                                                                   |
| Infrastructure         | `tofu -chdir=infra fmt -check -recursive`, `tofu -chdir=infra init -backend=false -input=false`, `tofu -chdir=infra validate` |

## Deployment

Delivery is handled by GitHub Actions:

- Pull requests target `staging`.
- A push to `staging` runs quality checks, builds/scans/attests one container image, deploys to Cloud Run staging, and smoke-tests it.
- A published GitHub release promotes the already verified staging digest to production. Production uses a protected environment, no-traffic candidate smoke tests, canary promotion, and rollback handling.

Release tags must match `package.json`:

```text
package.json version 2.8.1 -> release tag v2.8.1
```

Do not reuse or move release tags. Publish a new patch version if a release needs correction.

## Runtime Health

| Endpoint       | Purpose                          |
| -------------- | -------------------------------- |
| `/healthz`     | Process health.                  |
| `/readyz`      | Runtime configuration readiness. |
| `/dependencyz` | Supabase dependency readiness.   |

The deployment smoke test also checks localized public pages and security headers.

## Infrastructure

Cloud Run services, Artifact Registry, IAM, Secret Manager bindings, Workload Identity Federation, monitoring, and alerts live in `infra/` and are managed with OpenTofu.

After bootstrap, apply infrastructure changes only through the protected GitHub Actions infrastructure workflow. Do not create persistent console-only drift.

## License

MIT. See [LICENSE](LICENSE).
