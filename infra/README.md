# OpenTofu infrastructure

## Scope

- Artifact Registry Docker repository with deployment-safe retention
- Dedicated staging/production Cloud Run runtime identities
- Dedicated GitHub WIF pool and per-environment providers restricted by repository, immutable owner ID, exact environment subject, and staging-branch/semantic-tag ref
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

1. Remove the `qwik-aestheticlab` repository principal from the legacy shared `github-deployer` service account while retaining the other repositories' principals.
2. Do not remove shared-service-account project roles until its other repositories have migrated to dedicated identities.

## Deployment contract

OpenTofu owns service configuration and IAM. GitHub Actions owns only the container image and exact-revision traffic promotion. Image and traffic fields are intentionally ignored after bootstrap so a later plan cannot roll back a digest or promote an untested revision.

Production releases must reference a commit previously built and scan-verified from `staging`. If the immutable `verified-<commit>` tag is absent, release deployment fails without changing Cloud Run.
