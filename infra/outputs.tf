output "artifact_registry_image" {
  value = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.containers.repository_id}/${var.image_name}"
}

output "deployer_service_accounts" {
  value = { for environment, account in google_service_account.deployer : environment => account.email }
}

output "runtime_service_accounts" {
  value = { for environment, account in google_service_account.runtime : environment => account.email }
}

output "firestore_databases" {
  value = { for environment, database in google_firestore_database.site : environment => database.name }
}

output "service_urls" {
  value = { for environment, service in google_cloud_run_v2_service.web : environment => service.uri }
}

output "github_repository_variables" {
  value = {
    GCP_PROJECT    = var.project_id
    GCP_REGION     = var.region
    GAR_REPOSITORY = google_artifact_registry_repository.containers.repository_id
    IMAGE_NAME     = var.image_name
    STAGE_SERVICE  = var.staging_service_name
    PROD_SERVICE   = var.production_service_name
    PROD_URL       = "https://${var.production_uptime_host}"
  }
}

output "github_environment_variables" {
  value = {
    staging = {
      GCP_SERVICE_ACCOUNT      = google_service_account.deployer["staging"].email
      GCP_WORKLOAD_ID_PROVIDER = google_iam_workload_identity_pool_provider.github["staging"].name
    }
    production = {
      GCP_SERVICE_ACCOUNT      = google_service_account.deployer["production"].email
      GCP_WORKLOAD_ID_PROVIDER = google_iam_workload_identity_pool_provider.github["production"].name
    }
    infrastructure = {
      GCP_SERVICE_ACCOUNT      = google_service_account.iac.email
      GCP_WORKLOAD_ID_PROVIDER = google_iam_workload_identity_pool_provider.github["infrastructure"].name
    }
    infrastructure-plan = {
      GCP_SERVICE_ACCOUNT      = google_service_account.iac_plan.email
      GCP_WORKLOAD_ID_PROVIDER = google_iam_workload_identity_pool_provider.github["infrastructure-plan"].name
    }
  }
}
