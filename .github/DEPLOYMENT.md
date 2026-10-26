# Deployment Guide

## Delivery Model

```text
staging commit → verify → build/scan/attest once → Artifact Registry digest
               → staging no-traffic deploy → smoke test → staging traffic
GitHub release → resolve the verified digest for the release commit
               → production approval + independent watchdog
               → no-traffic deploy → smoke test → 10% canary
               → 5-minute bake + candidate 5xx and cross-revision asset 404 gates
               → canonical-domain check → 100% or automatic rollback
```

- GitHub Actions is the only automated delivery system; workflow definitions live in `.github/workflows/`.
- GCP authentication uses GitHub OIDC and Workload Identity Federation; no service-account keys are stored in GitHub.
- Container tags are lookup metadata. Scan success creates `verified-<full-commit-sha>`; Cloud Run deployments always use the immutable `sha256` digest.
- Production promotion never rebuilds. A published release must reference the exact commit already verified and deployed to staging; a separate runner reconciles an abandoned 10/90 canary.
- The protected `production` GitHub environment owns approval and environment-scoped configuration.
- During the canary bake the workflow smoke-tests the public URL so about 10% of requests reach the candidate, then reads the candidate revision's request logs. Any 5xx, or a failed log query, rolls traffic back to the previous revision. The gate also detects version skew: it counts previous-revision 404s for `/build/`, `/assets/` and `/static/` paths that the candidate serves as a hashed asset, a 200 cached `immutable` (checked through the candidate tag URL, at most 50 distinct paths), and rolls back above `CANARY_MAX_ASSET_404` (5). Arbitrary or probing asset URLs do not exist on the candidate and are not counted. For the Qwik-to-Go release (`v5.0.0`) this works in both directions: a Go page's `/static/` request that reaches the Qwik revision 404s and counts, while Qwik paths that reach the Go candidate get the `/build/` reload shim (`no-store`) or an `/assets/` redirect (301), never an immutable 200, so they do not count. Cloud Run session affinity (`session_affinity = true` in `infra/`) keeps each browser on one revision during the split; it is best effort, which is why the gate allows a few misses. The production deployer needs project `roles/logging.viewer` (declared in `infra/`); apply it through the `infrastructure` workflow before the first release that includes the gate.

Repository/environment configuration:

| Scope | Variables |
| --- | --- |
| Repository | `GCP_PROJECT`, `GCP_REGION`, `GAR_REPOSITORY`, `IMAGE_NAME`, `STAGE_SERVICE`, `PROD_SERVICE`, `PROD_URL` |
| Each environment | `GCP_WORKLOAD_ID_PROVIDER`, `GCP_SERVICE_ACCOUNT` |

Store configuration at the narrowest applicable scope. WIF resource names and service-account emails are identifiers, not secrets; no private key is stored in GitHub.
Store only non-sensitive Monitoring channel resource names in the repository variable `MONITORING_NOTIFICATION_CHANNEL_IDS` as a JSON list. Keep recipient addresses inside GCP notification channels so they never enter GitHub, OpenTofu plans, or state. Supabase runtime keys remain only in GCP Secret Manager and are never copied into GitHub Actions.

## Container

- `Dockerfile` builds the Go site (`cmd/web`, Phase 8 of `plans/go-datastar-scale-to-zero.md`): a Bun stage builds the hashed static assets, a Go stage embeds them into one static binary, and the runtime is `gcr.io/distroless/static-debian12:nonroot` with only that binary.
- The runtime is non-root, exposes port `3000`, and starts `/web`. Revisions built from the Qwik image stay available as rollback targets until Phase 9c.
- PR CI builds, runs, and scans the production container. Delivery generates an SBOM and provenance attestation, scans the exact published digest, and gates deployment on the result; production verifies provenance and re-scans before deployment.
- Images are stored in the regional Google Artifact Registry repository managed by OpenTofu.

Local verification:

```bash
bun run verify
make ci
docker build --tag aestheticlab:local .
docker run --rm --publish 3000:3000 aestheticlab:local
curl --fail http://localhost:3000/healthz
```

## Quality Gates

- `.github/workflows/quality.yml` runs on every pull request and `staging` push.
- It runs non-mutating Biome checks, type checking, unit tests, a production build, Chromium E2E smoke tests, a high-severity dependency audit, and full-history secret scanning.
- `.github/workflows/deploy.yml` pins third-party actions to commit SHAs and blocks deployment when Trivy finds a fixed high or critical image vulnerability.

## Cloud Run

- Staging and production are separate services with dedicated runtime service accounts and Secret Manager secrets. They currently share one read-only Supabase project protected by RLS.
- Runtime identities receive access only to the named Secret Manager secret, never project-wide secret roles.
- Deployments create a revision with no traffic, validate the Supabase dependency and localized pages through a temporary tagged URL, remove the tag after validation, then migrate traffic; Cloud Run startup and liveness probes use the process-only `/healthz` endpoint.
- Startup/liveness probes, scaling, concurrency, resource limits, labels, and environment variables are declared in `infra/`.
- Production supports multiple instances; tune concurrency and memory from monitoring data, not by console drift.

Required runtime configuration:

| Name | Source | Notes |
| --- | --- | --- |
| `FIRESTORE_DATABASE` | Cloud Run environment | `site-staging` or `site-production`; without it the Go site serves its embedded content snapshot and `/dependencyz` answers 503 |
| `GOOGLE_CLOUD_PROJECT` | Cloud Run environment | Saves the Firestore client a metadata-server lookup on a cold start |
| `SUPABASE_URL` | Cloud Run environment | Qwik only; kept until Phase 9b so a Qwik image still starts |
| `SUPABASE_KEY` | Environment-specific Secret Manager secret | Qwik only, as above; publishable/anon key injected at runtime; `service_role` and `sb_secret_*` keys fail readiness |

## Infrastructure as Code

- `infra/` is canonical for Artifact Registry, Cloud Run, IAM, Workload Identity Federation, secrets access, uptime checks, and alerts.
- Use a remote, versioned, encrypted state backend with locking; do not use local state for production resources.
- Pin OpenTofu and provider versions; commit `.terraform.lock.hcl`.
- Pull requests validate OpenTofu. Post-bootstrap plans and applies run only through `.github/workflows/infrastructure.yml`; a read-only planning identity creates the immutable plan, while the privileged identity is available only after protected `infrastructure` approval.
- Review plans for replacement, IAM expansion, public access, and secret exposure.
- Import existing resources before the first apply; never recreate production merely to bring it under management.

Typical local checks (follow `infra/README.md` for backend/environment inputs):

```bash
tofu -chdir=infra fmt -check -recursive
tofu -chdir=infra init -backend=false
tofu -chdir=infra validate
```

## Caching

The localized route shell is configured in `src/routes/[...lang]/layout.tsx`:

```tsx
cacheControl({
  staleWhileRevalidate: 60 * 10,
  maxAge: 60,
});
```

- Dynamic HTML is fresh for 60 seconds, then eligible for stale-while-revalidate for 10 minutes. The window stays short because a new revision does not serve the previous revision's hashed chunks.
- Hashed Vite assets use long-lived immutable caching.
- Translation changes require a rebuild because Qwik Speak inlines translations.

## Release

1. Merge a verified change to `staging` and confirm the staging deployment and smoke test passed.
2. Set `package.json` to the release version in the release change and run `bun run verify`.
3. Create an annotated semantic-version tag for that exact commit and push it.
4. Publish the GitHub release from the tag; the release-published workflow resolves the existing digest.
5. Approve the protected production environment after reviewing artifact identity and deployment metadata.
6. Confirm production smoke tests, traffic migration, uptime, errors, latency, and revision health.

Use `gh release create v<x.y.z> --verify-tag --title "v<x.y.z>" --generate-notes` after the tag is available remotely.

## Rollback

Prefer an immediate traffic rollback; it does not rebuild or mutate the failed revision:

```bash
gcloud run revisions list --service SERVICE --region REGION
gcloud run services update-traffic SERVICE \
  --region REGION \
  --to-revisions PREVIOUS_REVISION=100
```

- Record the incident and reconcile the desired image digest in OpenTofu/delivery configuration.
- Do not overwrite or reuse release tags.

## Operational Checks

- OpenTofu manages health/dependency uptime checks plus availability, 5xx, p95 latency, instance-saturation, memory-limit, structured Supabase-failure, and unexpected-production-mutation alerts. Keep high-frequency uptime checks on lightweight endpoints; localized page checks belong in deployment smoke tests.
- The 5xx and Supabase-failure alerts fire on counts within 5 minutes (`server_error_alert_threshold`, `supabase_failure_alert_threshold`), not on single events. The mutation alert reads Admin Activity audit logs for v1 (`ReplaceService`) and v2 (`UpdateService`) service changes, creates, deletes, and IAM changes by anyone other than the production deployer or IaC identity.
- OpenTofu does not pin Cloud Run revision names. After any CI deploy, an infrastructure plan shows both `google_cloud_run_v2_service.web` instances as updated in place, with `template.revision` changing from the CI `candidate-*` name to unset. This is expected: applying it creates a Cloud Run-named revision from the IaC template on the live image digest, and traffic stays on the CI-promoted revision (`traffic` and the image are in `ignore_changes`). Treat any other Cloud Run attribute change in the plan as a real change to review. Template changes reach serving traffic with the next CI deploy, which copies the service template.
- Artifact Registry keeps the newest `artifact_keep_recent_versions` images and deletes images older than `artifact_delete_older_than_days`. Cleanup starts in dry-run (`artifact_cleanup_dry_run = true`); review the cleanup logs, then set it to `false` to enforce.
- Candidate deployment smoke tests require readiness, localized content, and security headers on `/en-BE/` and `/fr-BE/pricelist/` before traffic migration.
- Smoke tests also request every optimized image variant referenced by those pages and require an image response. Identical image sources must use one canonical import so client and server builds cannot deduplicate to different filenames; cached legacy lazer1 URLs resolve to the same-hash universal asset.
- Before production traffic splitting, preflight identifies broken legacy lazer1 URLs in the previous revision's own HTML. A URL is excluded from mixed-traffic image checks and cross-revision counting only when the candidate repairs it with byte-identical data already served by the old revision under the canonical universal URL. Candidate and final smoke tests verify every image without exclusions. Newly missing assets and changed image bytes remain subject to the existing gate.
- Inspect Cloud Run revision logs and monitoring before shifting traffic.
- Treat a passing `/healthz` as process health only; localized smoke tests validate the dependency path.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Release cannot resolve an image | Tag commit must match a successful staging artifact/deployment record |
| New revision fails before traffic | Container logs, probes, secret access, runtime service-account IAM |
| Supabase data missing | Environment URL, secret version binding, loader error logs |
| Stale content after update | Dynamic response may remain fresh for 300 seconds |
| IaC proposes unexpected replacement | Provider/version drift, imported resource identity, environment state selection |
