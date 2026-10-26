locals {
  services = {
    staging = {
      name                    = var.staging_service_name
      min_instances           = var.staging_min_instances
      max_instances           = var.staging_max_instances
      supabase_url            = var.staging_supabase_url
      supabase_secret_id      = var.staging_supabase_secret_id
      supabase_secret_version = var.staging_supabase_secret_version
      firestore_database      = "site-staging"
    }
    production = {
      name                    = var.production_service_name
      min_instances           = var.production_min_instances
      max_instances           = var.production_max_instances
      supabase_url            = var.production_supabase_url
      supabase_secret_id      = var.production_supabase_secret_id
      supabase_secret_version = var.production_supabase_secret_version
      firestore_database      = "site-production"
    }
  }

  required_apis = toset([
    "artifactregistry.googleapis.com",
    "cloudresourcemanager.googleapis.com",
    "firestore.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "logging.googleapis.com",
    "monitoring.googleapis.com",
    "run.googleapis.com",
    "secretmanager.googleapis.com",
    "sts.googleapis.com",
  ])

  github_provider_conditions = {
    staging             = "assertion.repository == '${var.github_repository}' && assertion.repository_owner_id == '${var.github_repository_owner_id}' && assertion.sub == 'repo:${var.github_repository}:environment:staging' && assertion.ref == 'refs/heads/staging'"
    production          = "assertion.repository == '${var.github_repository}' && assertion.repository_owner_id == '${var.github_repository_owner_id}' && assertion.sub == 'repo:${var.github_repository}:environment:production' && assertion.ref.matches('^refs/tags/v[0-9]+[.][0-9]+[.][0-9]+$')"
    infrastructure      = "assertion.repository == '${var.github_repository}' && assertion.repository_owner_id == '${var.github_repository_owner_id}' && assertion.sub == 'repo:${var.github_repository}:environment:infrastructure' && assertion.ref == 'refs/heads/staging'"
    infrastructure-plan = "assertion.repository == '${var.github_repository}' && assertion.repository_owner_id == '${var.github_repository_owner_id}' && assertion.sub == 'repo:${var.github_repository}:environment:infrastructure-plan' && assertion.ref == 'refs/heads/staging'"
  }

  github_provider_display_names = {
    staging             = "Aesthetic Lab staging"
    production          = "Aesthetic Lab production"
    infrastructure      = "Aesthetic Lab infrastructure"
    infrastructure-plan = "Aesthetic Lab infra plan"
  }

  # roles/datastore.owner is the only predefined role that can create
  # Firestore databases. It grants no more than this account's
  # projectIamAdmin role already lets it grant itself.
  iac_project_roles = toset([
    "roles/artifactregistry.admin",
    "roles/datastore.owner",
    "roles/iam.serviceAccountAdmin",
    "roles/iam.workloadIdentityPoolAdmin",
    "roles/logging.configWriter",
    "roles/monitoring.editor",
    "roles/resourcemanager.projectIamAdmin",
    "roles/run.admin",
    "roles/secretmanager.admin",
    "roles/serviceusage.serviceUsageAdmin",
  ])

  iac_plan_project_roles = toset([
    "roles/artifactregistry.reader",
    "roles/iam.securityReviewer",
    "roles/iam.workloadIdentityPoolViewer",
    "roles/monitoring.viewer",
    "roles/secretmanager.viewer",
    "roles/serviceusage.serviceUsageViewer",
    "roles/viewer",
  ])
}

check "environment_isolation" {
  assert {
    condition     = var.staging_supabase_secret_id != var.production_supabase_secret_id
    error_message = "Staging and production must use different Secret Manager secrets, even when they share one Supabase project."
  }
}

check "site_databases" {
  assert {
    condition = alltrue([
      for env in values(local.services) : startswith(env.firestore_database, "site-")
    ]) && local.services.staging.firestore_database != local.services.production.firestore_database
    error_message = "Each environment needs its own site-* Firestore database; (default) and aestheticlab-income belong to other apps."
  }
}

check "initial_image_location" {
  assert {
    condition = startswith(
      var.initial_image,
      "${var.region}-docker.pkg.dev/${var.project_id}/${var.artifact_registry_repository}/${var.image_name}@sha256:",
    )
    error_message = "initial_image must reference this project's configured Artifact Registry image."
  }
}

resource "google_project_service" "required" {
  for_each = local.required_apis

  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}

resource "google_artifact_registry_repository" "containers" {
  location      = var.region
  repository_id = var.artifact_registry_repository
  description   = "Aesthetic Lab deployable containers"
  format        = "DOCKER"

  # Every staging push adds an image. Keep the newest images (current and rollback
  # revisions) and delete only old images beyond that window.
  cleanup_policy_dry_run = var.artifact_cleanup_dry_run

  cleanup_policies {
    id     = "keep-recent-images"
    action = "KEEP"
    most_recent_versions {
      keep_count = var.artifact_keep_recent_versions
    }
  }

  cleanup_policies {
    id     = "delete-old-images"
    action = "DELETE"
    condition {
      tag_state  = "ANY"
      older_than = "${var.artifact_delete_older_than_days * 24 * 60 * 60}s"
    }
  }

  depends_on = [google_project_service.required]
}

resource "google_secret_manager_secret" "supabase_key" {
  for_each = local.services

  secret_id           = each.value.supabase_secret_id
  deletion_protection = var.deletion_protection
  replication {
    auto {}
  }

  depends_on = [google_project_service.required]
}

resource "google_iam_workload_identity_pool" "github" {
  project                   = var.project_id
  workload_identity_pool_id = "aestheticlab-github"
  display_name              = "Aesthetic Lab GitHub Actions"

  depends_on = [google_project_service.required]
}

resource "google_iam_workload_identity_pool_provider" "github" {
  for_each = local.github_provider_conditions

  project                            = var.project_id
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-${each.key}"
  display_name                       = local.github_provider_display_names[each.key]

  attribute_mapping = {
    "google.subject"                = "assertion.sub"
    "attribute.repository"          = "assertion.repository"
    "attribute.repository_owner_id" = "assertion.repository_owner_id"
  }

  attribute_condition = each.value

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

resource "google_service_account" "runtime" {
  for_each = local.services

  account_id   = "aesthetic-${each.key == "production" ? "prod" : "stage"}-runtime"
  display_name = "Aesthetic Lab ${each.key} Cloud Run runtime"
}

resource "google_service_account" "deployer" {
  for_each = local.services

  account_id   = "aesthetic-${each.key == "production" ? "prod" : "stage"}-deployer"
  display_name = "Aesthetic Lab ${each.key} GitHub deployer"
}

resource "google_service_account" "iac" {
  account_id   = "aesthetic-infra-deployer"
  display_name = "Aesthetic Lab protected OpenTofu deployer"
}

resource "google_service_account" "iac_plan" {
  account_id   = "aesthetic-infra-planner"
  display_name = "Aesthetic Lab read-only OpenTofu planner"
}

resource "google_project_iam_member" "iac" {
  for_each = local.iac_project_roles

  project = var.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.iac.email}"
}

resource "google_project_iam_member" "iac_plan" {
  for_each = local.iac_plan_project_roles

  project = var.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.iac_plan.email}"
}

# The site's content (plans/go-datastar-scale-to-zero.md, docs/CONTENT.md).
# The project's other Firestore databases, (default) and aestheticlab-income,
# belong to other apps: OpenTofu never manages them, and the site's
# identities get only conditioned grants that cannot reach them.
resource "google_firestore_database" "site" {
  for_each = local.services

  name                        = each.value.firestore_database
  location_id                 = var.region
  type                        = "FIRESTORE_NATIVE"
  concurrency_mode            = "OPTIMISTIC"
  app_engine_integration_mode = "DISABLED"
  delete_protection_state     = "DELETE_PROTECTION_ENABLED"
  deletion_policy             = "ABANDON"

  depends_on = [google_project_service.required, google_project_iam_member.iac]
}

# Read-only, and only on the environment's own database: an unconditioned
# datastore role would reach every database in the project.
resource "google_project_iam_member" "runtime_firestore_reader" {
  for_each = local.services

  project = var.project_id
  role    = "roles/datastore.viewer"
  member  = "serviceAccount:${google_service_account.runtime[each.key].email}"

  condition {
    title       = "${each.key}-site-database-only"
    description = "Read the ${google_firestore_database.site[each.key].name} database only."
    expression  = "resource.name == \"projects/${var.project_id}/databases/${google_firestore_database.site[each.key].name}\""
  }
}

resource "google_secret_manager_secret_iam_member" "runtime_access" {
  for_each = local.services

  secret_id = google_secret_manager_secret.supabase_key[each.key].id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.runtime[each.key].email}"
}

resource "google_service_account_iam_member" "github_wif" {
  for_each = local.services

  service_account_id = google_service_account.deployer[each.key].name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principal://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/subject/repo:${var.github_repository}:environment:${each.key}"

  depends_on = [google_iam_workload_identity_pool_provider.github]
}

resource "google_service_account_iam_member" "github_wif_iac" {
  service_account_id = google_service_account.iac.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principal://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/subject/repo:${var.github_repository}:environment:infrastructure"

  depends_on = [google_iam_workload_identity_pool_provider.github]
}

resource "google_service_account_iam_member" "github_wif_iac_plan" {
  service_account_id = google_service_account.iac_plan.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principal://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/subject/repo:${var.github_repository}:environment:infrastructure-plan"

  depends_on = [google_iam_workload_identity_pool_provider.github]
}

resource "google_artifact_registry_repository_iam_member" "staging_deployer_writer" {
  project    = var.project_id
  location   = google_artifact_registry_repository.containers.location
  repository = google_artifact_registry_repository.containers.repository_id
  role       = "roles/artifactregistry.writer"
  member     = "serviceAccount:${google_service_account.deployer["staging"].email}"
}

resource "google_artifact_registry_repository_iam_member" "production_deployer_reader" {
  project    = var.project_id
  location   = google_artifact_registry_repository.containers.location
  repository = google_artifact_registry_repository.containers.repository_id
  role       = "roles/artifactregistry.reader"
  member     = "serviceAccount:${google_service_account.deployer["production"].email}"
}

# The production canary gate reads the candidate revision's request logs for 5xx
# responses before promotion; logs are read-only and data-access logs are excluded.
resource "google_project_iam_member" "production_deployer_log_viewer" {
  project = var.project_id
  role    = "roles/logging.viewer"
  member  = "serviceAccount:${google_service_account.deployer["production"].email}"
}

resource "google_service_account_iam_member" "deployer_act_as" {
  for_each = local.services

  service_account_id = google_service_account.runtime[each.key].name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.deployer[each.key].email}"
}

resource "google_service_account_iam_member" "iac_act_as" {
  for_each = local.services

  service_account_id = google_service_account.runtime[each.key].name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.iac.email}"
}

resource "google_cloud_run_v2_service" "web" {
  for_each = local.services

  name                = each.value.name
  location            = var.region
  deletion_protection = var.deletion_protection
  ingress             = "INGRESS_TRAFFIC_ALL"

  # Service-level scaling applies across all revisions and overrides the
  # template's minimum. Production had min 1 here, set outside OpenTofu, which
  # kept an instance warm after v5.0.0 set the revisions' minimum to 0.
  scaling {
    min_instance_count = each.value.min_instances
    max_instance_count = each.value.max_instances
  }

  template {
    service_account                  = google_service_account.runtime[each.key].email
    max_instance_request_concurrency = var.container_concurrency
    execution_environment            = "EXECUTION_ENVIRONMENT_GEN2"
    # Keep each browser on one revision during canary splits. Hashed /build/ and
    # /assets/ files exist only in the revision that rendered the page; without
    # affinity, a 90/10 split routes follow-up chunk requests to the other revision
    # (404s, unstyled pages). Best effort: affinity breaks if the instance goes away.
    session_affinity = true

    scaling {
      min_instance_count = each.value.min_instances
      max_instance_count = each.value.max_instances
    }

    containers {
      image = var.initial_image

      ports {
        container_port = 3000
      }

      resources {
        limits = {
          cpu = "1"
          # The Go site peaks at about 16 MiB; 512 MiB is the minimum for the
          # second-generation execution environment.
          memory = "512Mi"
        }
        cpu_idle          = true
        startup_cpu_boost = true
      }

      # The Go site's content (cmd/web). The project is set so the client
      # skips a metadata-server lookup on a cold start. The Supabase pair
      # stays until Phase 9b, so a deploy of either the Go or the Qwik image
      # starts.
      env {
        name  = "FIRESTORE_DATABASE"
        value = google_firestore_database.site[each.key].name
      }

      env {
        name  = "GOOGLE_CLOUD_PROJECT"
        value = var.project_id
      }

      env {
        name  = "SUPABASE_URL"
        value = each.value.supabase_url
      }

      env {
        name = "SUPABASE_KEY"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.supabase_key[each.key].secret_id
            version = each.value.supabase_secret_version
          }
        }
      }

      # Cloud Run's default TCP startup probe: one attempt that waits up to 240 s
      # for the port, so a cold start is routed as soon as the server listens. The
      # 1 s HTTP probe it replaces missed the port on its first attempt and added
      # about 0.7 s to every cold start (plan, Phase 8). Both servers are ready to
      # serve once they listen; /healthz stays process-only for liveness.
      startup_probe {
        initial_delay_seconds = 0
        timeout_seconds       = 240
        period_seconds        = 240
        failure_threshold     = 1
        tcp_socket {
          port = 3000
        }
      }

      liveness_probe {
        initial_delay_seconds = 10
        timeout_seconds       = 3
        period_seconds        = 10
        failure_threshold     = 3
        http_get {
          path = "/healthz"
          port = 3000
        }
      }
    }
  }

  # template.revision is deliberately not ignored. CI names every revision
  # (--revision-suffix); ignoring it made OpenTofu re-send that existing name with
  # a changed template, which Cloud Run rejects with HTTP 409. Leaving it unset lets
  # Cloud Run name IaC revisions. Image and traffic stay CI-owned, so an IaC revision
  # reuses the live digest and receives no traffic until the next promotion.
  lifecycle {
    ignore_changes = [
      client,
      client_version,
      template[0].containers[0].image,
      traffic,
    ]
  }

  depends_on = [
    google_project_service.required,
    google_project_iam_member.runtime_firestore_reader,
    google_secret_manager_secret_iam_member.runtime_access,
    google_service_account_iam_member.iac_act_as,
  ]
}

resource "google_cloud_run_v2_service_iam_member" "public" {
  for_each = var.allow_public_access ? local.services : {}

  project  = var.project_id
  location = google_cloud_run_v2_service.web[each.key].location
  name     = google_cloud_run_v2_service.web[each.key].name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

resource "google_cloud_run_v2_service_iam_member" "deployer" {
  for_each = local.services

  project  = var.project_id
  location = google_cloud_run_v2_service.web[each.key].location
  name     = google_cloud_run_v2_service.web[each.key].name
  role     = "roles/run.developer"
  member   = "serviceAccount:${google_service_account.deployer[each.key].email}"
}

resource "google_monitoring_alert_policy" "server_errors" {
  display_name          = "Aesthetic Lab production 5xx responses"
  combiner              = "OR"
  notification_channels = var.notification_channel_ids

  conditions {
    display_name = "5xx responses exceed threshold in 5 minutes"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${var.production_service_name}\" AND metric.type = \"run.googleapis.com/request_count\" AND metric.labels.response_code_class = \"5xx\""
      comparison      = "COMPARISON_GT"
      threshold_value = var.server_error_alert_threshold
      duration        = "0s"

      aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_SUM"
        cross_series_reducer = "REDUCE_SUM"
        group_by_fields      = ["resource.labels.service_name"]
      }

      trigger {
        count = 1
      }
    }
  }

  alert_strategy {
    auto_close = "604800s"
  }
}

# No uptime checks: a probe every minute would keep an instance warm and stop
# the service scaling to zero (plan, Decisions). Alerts fire on real traffic,
# and every deploy smoke-tests /readyz, /dependencyz and the pages.

resource "google_monitoring_alert_policy" "latency" {
  display_name          = "Aesthetic Lab production p95 latency"
  combiner              = "OR"
  notification_channels = var.notification_channel_ids

  conditions {
    display_name = "p95 request latency exceeds two seconds"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${var.production_service_name}\" AND metric.type = \"run.googleapis.com/request_latencies\""
      comparison      = "COMPARISON_GT"
      threshold_value = 2000
      duration        = "300s"

      aggregations {
        alignment_period     = "60s"
        per_series_aligner   = "ALIGN_PERCENTILE_95"
        cross_series_reducer = "REDUCE_MAX"
        group_by_fields      = ["resource.labels.service_name"]
      }

      trigger {
        count = 1
      }
    }
  }

  alert_strategy {
    auto_close = "604800s"
  }
}

resource "google_monitoring_alert_policy" "instance_saturation" {
  display_name          = "Aesthetic Lab production instance saturation"
  combiner              = "OR"
  notification_channels = var.notification_channel_ids

  conditions {
    display_name = "Active instances at configured maximum"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${var.production_service_name}\" AND metric.type = \"run.googleapis.com/container/instance_count\" AND metric.labels.state = \"active\""
      comparison      = "COMPARISON_GT"
      threshold_value = var.production_max_instances - 1
      duration        = "300s"

      aggregations {
        alignment_period     = "60s"
        per_series_aligner   = "ALIGN_MAX"
        cross_series_reducer = "REDUCE_SUM"
        group_by_fields      = ["resource.labels.service_name"]
      }

      trigger {
        count = 1
      }
    }
  }

  alert_strategy {
    auto_close = "604800s"
  }
}

resource "google_monitoring_alert_policy" "runtime_failure" {
  display_name          = "Aesthetic Lab production runtime failure"
  combiner              = "OR"
  notification_channels = var.notification_channel_ids

  conditions {
    display_name = "Cloud Run reports memory, startup, or termination failure"
    condition_matched_log {
      filter = "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${var.production_service_name}\" AND (textPayload:\"memory limit\" OR textPayload:\"failed to start and listen\" OR textPayload:\"container instance was terminated\" OR jsonPayload.message:\"memory limit\")"
    }
  }

  alert_strategy {
    notification_rate_limit {
      period = "300s"
    }
    auto_close = "604800s"
  }
}

resource "google_logging_metric" "content_failures" {
  name        = "aestheticlab_production_content_failures"
  description = "Production content failures: Go content refresh failures and snapshot fallbacks, and Qwik Supabase loader failures until Qwik is removed."
  filter      = "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${var.production_service_name}\" AND (jsonPayload.message=\"content_refresh_failed\" OR jsonPayload.message=\"content_snapshot_served\" OR jsonPayload.message=\"supabase_fetch_failed\" OR jsonPayload.message=\"supabase_configuration_rejected\")"

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"
  }

  depends_on = [google_project_service.required]
}

moved {
  from = google_monitoring_alert_policy.supabase_failure
  to   = google_monitoring_alert_policy.content_failure
}

resource "google_monitoring_alert_policy" "content_failure" {
  display_name          = "Aesthetic Lab production content failure"
  combiner              = "OR"
  notification_channels = var.notification_channel_ids

  conditions {
    # A cold instance that can't reach Firestore logs a failed refresh and then
    # serves the snapshot, and a Qwik render logs up to four loader failures,
    # so alert on a sustained count rather than single events.
    display_name = "Content failures exceed threshold in 5 minutes"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND metric.type = \"logging.googleapis.com/user/${google_logging_metric.content_failures.name}\""
      comparison      = "COMPARISON_GT"
      threshold_value = var.content_failure_alert_threshold
      duration        = "0s"

      aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_SUM"
        cross_series_reducer = "REDUCE_SUM"
      }

      trigger {
        count = 1
      }
    }
  }

  alert_strategy {
    auto_close = "604800s"
  }
}

resource "google_monitoring_alert_policy" "startup_latency" {
  display_name          = "Aesthetic Lab production slow cold starts"
  combiner              = "OR"
  notification_channels = var.notification_channel_ids

  conditions {
    # With min instances 0 most visits after a quiet spell start an instance,
    # so startup time is part of the page load. Windows with no starts report
    # nothing and never alert.
    display_name = "Container startup p95 above target in 15 minutes"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${var.production_service_name}\" AND metric.type = \"run.googleapis.com/container/startup_latencies\""
      comparison      = "COMPARISON_GT"
      threshold_value = var.startup_latency_alert_ms
      duration        = "0s"

      aggregations {
        alignment_period     = "900s"
        per_series_aligner   = "ALIGN_PERCENTILE_95"
        cross_series_reducer = "REDUCE_MAX"
        group_by_fields      = ["resource.labels.service_name"]
      }

      trigger {
        count = 1
      }
    }
  }

  alert_strategy {
    auto_close = "604800s"
  }
}

resource "google_monitoring_alert_policy" "firestore_reads" {
  display_name          = "Aesthetic Lab production Firestore reads"
  combiner              = "OR"
  notification_channels = var.notification_channel_ids

  conditions {
    # Normal traffic reads well under 10k documents a day (plan, Firestore Read
    # Model). This catches a refresh loop or a broken cache, not growth.
    display_name = "Document reads exceed the hourly budget"
    condition_threshold {
      filter          = "resource.type = \"firestore.googleapis.com/Database\" AND resource.labels.database_id = \"${google_firestore_database.site["production"].name}\" AND metric.type = \"firestore.googleapis.com/document/read_ops_count\""
      comparison      = "COMPARISON_GT"
      threshold_value = var.firestore_read_alert_per_hour
      duration        = "0s"

      aggregations {
        alignment_period     = "3600s"
        per_series_aligner   = "ALIGN_SUM"
        cross_series_reducer = "REDUCE_SUM"
      }

      trigger {
        count = 1
      }
    }
  }

  alert_strategy {
    auto_close = "604800s"
  }
}

resource "google_monitoring_alert_policy" "unexpected_production_mutation" {
  display_name          = "Aesthetic Lab unexpected production Cloud Run mutation"
  combiner              = "OR"
  notification_channels = var.notification_channel_ids

  conditions {
    display_name = "Production changed outside delivery or protected IaC identities"
    condition_matched_log {
      # v1 (gcloud) logs ReplaceService, v2 logs UpdateService; resourceName covers both
      # "namespaces/<project>/services/<name>" and "projects/.../services/<name>".
      filter = "logName=\"projects/${var.project_id}/logs/cloudaudit.googleapis.com%2Factivity\" AND protoPayload.serviceName=\"run.googleapis.com\" AND protoPayload.resourceName:\"services/${var.production_service_name}\" AND (protoPayload.methodName:\"Services.CreateService\" OR protoPayload.methodName:\"Services.ReplaceService\" OR protoPayload.methodName:\"Services.UpdateService\" OR protoPayload.methodName:\"Services.DeleteService\" OR protoPayload.methodName:\"Services.SetIamPolicy\") AND NOT protoPayload.authenticationInfo.principalEmail=\"${google_service_account.deployer["production"].email}\" AND NOT protoPayload.authenticationInfo.principalEmail=\"${google_service_account.iac.email}\""
    }
  }

  alert_strategy {
    notification_rate_limit {
      period = "300s"
    }
    auto_close = "604800s"
  }
}
