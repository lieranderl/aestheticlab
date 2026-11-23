# OpenTofu infrastructure

## Scope

- Artifact Registry Docker repository with deployment-safe retention
- Dedicated staging/production Cloud Run runtime identities
- Dedicated GitHub WIF pool and per-environment providers restricted by immutable repository and owner IDs, the exact immutable environment subject, and staging-branch/semantic-tag ref
- Separate staging/production GitHub deployers and runtime identities
- Separate read-only planner and protected privileged apply identity for saved-plan OpenTofu automation
- The Go site's Firestore databases `site-staging` and `site-production` (delete-protected), each readable only by its environment's runtime through a conditioned `roles/datastore.viewer`; the project's other databases are not managed here
- Cloud Run v2 services, probes, scaling, and public invocation
- Production 5xx, p95 latency, instance saturation, runtime-failure, content-failure, slow-cold-start, Firestore-read, and unexpected-mutation alerts; no uptime checks, so both services scale to zero
- Artifact Registry cleanup policy (dry-run by default) keeping recent rollback images

## Bootstrap

1. Create a remote GCS state bucket with versioning, uniform access, soft delete, and bucket-scoped `roles/storage.objectAdmin` for the backend identity. Do not use a bucket retention policy: it can prevent deletion of the lock object.
2. Authenticate the backend/bootstrap identity, then run `tofu init -backend-config=bucket=BUCKET -backend-config=prefix=aestheticlab`.
3. Copy `terraform.tfvars.example` to an untracked `terraform.tfvars`, replace the placeholder digest, and set all required values.
4. Run `tofu plan -lock-timeout=60s -out=tfplan.bootstrap`, cancel without applying, and verify the GCS `.tflock` object is released. Resolve backend IAM/locking before any infrastructure write.
5. For a fresh project, bootstrap APIs and Artifact Registry first: `tofu apply -target=google_project_service.required -target=google_artifact_registry_repository.containers`.
6. Push an immutable bootstrap image and set `initial_image` to its GAR digest. Existing services must be imported with their current immutable GAR digest.
7. Run `tofu plan -out=tfplan`, review replacement/IAM/public-access changes, then apply the initial saved plan with the bootstrap identity.
8. Bootstrap at least one Monitoring notification channel outside OpenTofu state, then add its non-sensitive full resource name to the `MONITORING_NOTIFICATION_CHANNEL_IDS` repository variable as a JSON list. Keep recipient addresses out of GitHub, plans, and state. Add `github_repository_variables` plus the state and image values below as repository variables. Add each `github_environment_variables` output to its matching environment.
9. Grant the `infrastructure-plan` service account bucket-scoped `roles/storage.objectViewer`; grant the protected `infrastructure` service account bucket-scoped `roles/storage.objectAdmin` for state locking and writes.
10. Configure required reviewers on `production` and `infrastructure`; restrict `infrastructure-plan` and `infrastructure` to `staging`; require PR/security checks on `staging` before enabling deployer variables.

After bootstrap, `.github/workflows/infrastructure.yml` is the only apply path. A read-only `infrastructure-plan` identity creates an immutable saved plan without taking or mutating the state lock. The privileged identity is unavailable until the protected `infrastructure` environment approves applying that exact artifact.

GitHub signs this repository's tokens with an immutable subject, `repo:OWNER@OWNER_ID/NAME@REPOSITORY_ID:environment:ENVIRONMENT`. It still contains the repository name, so a rename or transfer locks every provider out, the infrastructure workflow's included. Check the current prefix with `gh api repos/OWNER/NAME/actions/oidc/customization/sub`.

Break-glass, only when the workflow cannot authenticate: merge the fix to `staging`, then have the bootstrap identity run `tofu plan -lock-timeout=60s -out=tfplan` on that commit, review it, and `tofu apply tfplan`. Afterwards, dispatch the infrastructure workflow and confirm its plan shows no changes.

Infrastructure workflow variables:

| Variable | Value |
| --- | --- |
| `GCP_WORKLOAD_ID_PROVIDER`, `GCP_SERVICE_ACCOUNT` | Environment-specific outputs for `infrastructure-plan` and `infrastructure` |
| `TF_STATE_BUCKET`, `TF_STATE_PREFIX` | Remote GCS backend coordinates |
| `GCP_PROJECT`, `GCP_REGION` | Target project and region |
| `INITIAL_IMAGE` | Immutable GAR digest used only for bootstrap/import reconciliation |
| `MONITORING_NOTIFICATION_CHANNEL_IDS` | JSON list of existing Monitoring notification channel resource names |

Notification channel recipients are sensitive bootstrap data. OpenTofu manages alert policies and references channels by resource name, but deliberately does not manage notification-channel recipient configuration.

## Existing resources

Import existing resources before the first plan; do not recreate live services. Examples:

```sh
tofu import google_artifact_registry_repository.containers projects/PROJECT/locations/REGION/repositories/REPOSITORY
tofu import 'google_cloud_run_v2_service.web["staging"]' projects/PROJECT/locations/REGION/services/STAGING_SERVICE
tofu import 'google_cloud_run_v2_service.web["production"]' projects/PROJECT/locations/REGION/services/PRODUCTION_SERVICE
```

Import service accounts and IAM resources as needed, then reconcile the plan. Set `deletion_protection = false` only during initial import if the provider requires it; restore it before apply.

After both environment-scoped deployers have completed a successful deployment:

1. The `lieranderl/qwik-aestheticlab` principal (the repository's name before Phase 10c) was removed from the legacy shared `github-deployer` service account on 2026-10-31; the other repositories' principals remain.
2. Do not remove shared-service-account project roles until its other repositories have migrated to dedicated identities.

## Chat assistant

`chat_environments` (default `["staging", "production"]`) sets `CHAT_ENABLED` and grants those runtime accounts the custom role `aestheticLabGeminiCaller`, which holds only `aiplatform.endpoints.predict`: the permission Google checks for prompt requests (`generateContent`) to Gemini on Vertex AI. Not `roles/aiplatform.user`, which would let the public runtime manage every Vertex AI resource in the project. `chat_model` and `chat_location` (`eu`) choose the model and keep processing in the EU.

The infrastructure deployer holds `roles/iam.roleAdmin` to manage that role. The apply that introduces it grants the role to the deployer first; if IAM hasn't propagated yet, creating the custom role fails with a 403 on `iam.roles.create` and the runtime keeps its old binding (the new one is created before the old one is removed). Re-run the infrastructure workflow to finish.

To confirm the chat still reaches Gemini under the runtime identity after an apply, and before promoting:

1. Check the binding: `gcloud projects get-iam-policy PROJECT --flatten=bindings --filter='bindings.members:aesthetic-stage-runtime@' --format='value(bindings.role)'` lists `projects/PROJECT/roles/aestheticLabGeminiCaller` and no `roles/aiplatform.user`.
2. Ask the staging chat one question that needs the model (not a canned answer) and get a real reply; IAM changes can take a few minutes to apply.
3. In Cloud Logging for the staging service, there is no `PERMISSION_DENIED` or `aiplatform.endpoints.predict` error from the chat after that request.
4. Optionally, as someone allowed to impersonate the runtime account, call the model directly: `curl -H "Authorization: Bearer $(gcloud auth print-access-token --impersonate-service-account=aesthetic-stage-runtime@PROJECT.iam.gserviceaccount.com)" -H 'Content-Type: application/json' -d '{"contents":[{"role":"user","parts":[{"text":"Say OK"}]}]}' https://aiplatform.eu.rep.googleapis.com/v1/projects/PROJECT/locations/eu/publishers/google/models/MODEL:generateContent` answers 200. This is a billed call outside the chat's ledger, so keep it to one.

`availability_calendars` maps staff calendars to staff IDs (`<calendarId>|<staffId>;…`). It is `sensitive` and never in the repository: CI reads it from the `AVAILABILITY_CALENDARS` repository secret (`TF_VAR_availability_calendars`); locally, put it in the git-ignored `terraform.tfvars`. Each calendar must be shared with the environment's runtime account at "See only free/busy (hide details)". Empty turns free times off: the chat offers the Book button instead.

Settings changes reach traffic with the next deploy, which copies the service's current template.

### Spending caps

Google Cloud budgets only alert; they never stop spending, and billing data arrives hours late. The chat's spending is therefore capped in the app: every reply's tokens are priced (Vertex AI's prices plus 25%) and added to a ledger in the `chat-usage` Firestore database, shared by every environment, and the chat stops calling models at `chat_monthly_budget_eur` (default €9) or `chat_daily_budget_eur` (default €0.50). If the ledger can't be read, the chat stays off. The runtime accounts may write that database only.

On top, `google_billing_budget.chat_models` is a €10 (`chat_budget_eur`) monthly budget on the billing account for the project's Vertex AI and Gemini API spending, alerting at 50%, 90%, 100% and a 100% forecast. The alert policies `chat_budget` (a cap reached, or the ledger failing) and `chat_volume` (more than `chat_replies_alert_per_hour` replies) notify the same channels.

The budget needs one-time access on the billing account, which only its Billing Admin can grant (OpenTofu can't grant itself access there):

1. In the console, **Billing → Account management → Permissions** for the AestheticLab account, grant `aesthetic-infra-deployer@PROJECT.iam.gserviceaccount.com` **Billing Account Costs Manager** and `aesthetic-infra-planner@PROJECT.iam.gserviceaccount.com` **Billing Account Viewer**.
2. Set the repository variable `GCP_BILLING_ACCOUNT` to the account ID (`gh variable set GCP_BILLING_ACCOUNT`). Until it is set, the budget is skipped and the plan stays clean.

#### Vertex AI spend cap (bootstrap, outside OpenTofu)

Unlike a budget, a [spend cap](https://docs.cloud.google.com/billing/docs/how-to/budgets-spend-caps) pauses new Vertex AI requests in the project once the month's estimated list-price cost passes its amount. It is the backstop if the app's ledger ever fails open. Enforcement can lag, in-flight requests finish and are billed, and while it is enforced the chat answers with its fixed reply. It covers exactly one project and one service, monthly, with fixed 50/80/100% alerts to billing admins and project owners, and can't be made from an existing alerts-only budget. The Google provider (8.12) has no spend-cap field, so a Billing Account Administrator creates it once, by hand, and OpenTofu doesn't manage it. The chat uses Vertex AI only, so the Gemini API (AI Studio) service needs no cap.

It exists for the AestheticLab account since 2026-11-23 (budget "Aesthetic Lab Vertex AI spend cap", €10). To recreate it, use the billing admin's own credentials; the token goes in a header file, never on the command line (`PROJECT_NUMBER` is the project's number, not its ID; keep the amount at `chat_budget_eur`):

```bash
curl -sS -X POST "https://billingbudgets.googleapis.com/v1/billingAccounts/BILLING_ACCOUNT/budgets" \
  -H @<(printf 'Authorization: Bearer %s\n' "$(gcloud auth print-access-token)") -H "x-goog-user-project: PROJECT_ID" -H "Content-Type: application/json" \
  -d '{"displayName": "Aesthetic Lab Vertex AI spend cap",
       "budgetFilter": {"projects": ["projects/PROJECT_NUMBER"], "services": ["services/C7E2-9256-1C43"], "creditTypesTreatment": "EXCLUDE_ALL_CREDITS", "calendarPeriod": "MONTH"},
       "amount": {"specifiedAmount": {"currencyCode": "EUR", "units": "10"}},
       "thresholdRules": [{"thresholdPercent": 0.5}, {"thresholdPercent": 0.8}, {"thresholdPercent": 1.0}],
       "notificationsRule": {"enableProjectLevelRecipients": true},
       "spendCap": {"inputState": "CONFIGURED"}}'
```

The response's `spendCap.outputState` is `CONFIGURED`; it becomes `ENFORCED` at the cap and lifts by itself on the 1st. To lift it early, use **Billing → Budgets & alerts → Edit budget → Lift spend cap** (service returns within about an hour); an enforced cap can't be edited or deleted until lifted.

## Deployment contract

OpenTofu owns service configuration and IAM. GitHub Actions owns only the container image and exact-revision traffic promotion. Image and traffic fields are intentionally ignored after bootstrap so a later plan cannot roll back a digest or promote an untested revision.

Production releases must reference a commit previously built and scan-verified from `staging`. If the immutable `verified-<commit>` tag is absent, release deployment fails without changing Cloud Run.
