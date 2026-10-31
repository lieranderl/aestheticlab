variable "project_id" {
  description = "Google Cloud project ID."
  type        = string
}

variable "region" {
  description = "Region for Artifact Registry and Cloud Run."
  type        = string
  default     = "europe-west1"
}

variable "artifact_registry_repository" {
  description = "Artifact Registry Docker repository ID."
  type        = string
  default     = "aestheticlab"
}

variable "image_name" {
  description = "Container image name within the repository."
  type        = string
  default     = "web"
}

variable "initial_image" {
  description = "Bootstrapping image; CI owns subsequent image updates by digest."
  type        = string

  validation {
    condition     = can(regex("@sha256:[0-9a-f]{64}$", var.initial_image))
    error_message = "initial_image must use an immutable sha256 digest."
  }
}

variable "staging_service_name" {
  type    = string
  default = "stage-aestheticlab-web"
}

variable "production_service_name" {
  type    = string
  default = "aestheticlab-web"
}

variable "production_uptime_host" {
  description = "Public production hostname, exported as the PROD_URL deployment variable."
  type        = string
  default     = "aestheticlab.be"
}

variable "github_repositories" {
  description = "GitHub repositories (owner/name) allowed to federate into the dedicated pool. Two names only while the repository is renamed (Phase 10c)."
  type        = list(string)
  default     = ["lieranderl/qwik-aestheticlab", "lieranderl/aestheticlab"]

  validation {
    condition     = length(var.github_repositories) > 0 && alltrue([for repository in var.github_repositories : can(regex("^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$", repository))])
    error_message = "List at least one repository, each as owner/name."
  }
}

variable "github_repository_owner_id" {
  description = "Immutable numeric GitHub owner ID used in the WIF provider condition."
  type        = string
  default     = "19622412"
}

variable "notification_channel_ids" {
  description = "Existing Monitoring notification channel resource names used by production alerts."
  type        = list(string)

  validation {
    condition = (
      length(var.notification_channel_ids) > 0 &&
      alltrue([
        for channel_id in var.notification_channel_ids :
        can(regex("^projects/[^/]+/notificationChannels/[^/]+$", channel_id))
      ])
    )
    error_message = "notification_channel_ids must contain at least one full Monitoring notification channel resource name."
  }
}

variable "allow_public_access" {
  description = "Grant unauthenticated Cloud Run invocation."
  type        = bool
  default     = true
}

variable "deletion_protection" {
  description = "Protect Cloud Run services from accidental deletion after bootstrap."
  type        = bool
  default     = true
}

variable "staging_min_instances" {
  type    = number
  default = 0
}

variable "staging_max_instances" {
  type    = number
  default = 2
}

variable "production_min_instances" {
  description = "Minimum production instances. 0 lets the service scale to zero between visits (plan Phase 9a)."
  type        = number
  default     = 0
}

variable "production_max_instances" {
  type    = number
  default = 3

  validation {
    condition     = var.production_max_instances >= 1
    error_message = "production_max_instances must be at least 1."
  }
}

variable "container_concurrency" {
  description = "Maximum concurrent requests per instance."
  type        = number
  default     = 40
}

variable "artifact_cleanup_dry_run" {
  description = "Log Artifact Registry cleanup decisions without deleting images. Set false after reviewing the dry-run logs."
  type        = bool
  default     = true
}

variable "artifact_keep_recent_versions" {
  description = "Newest image versions always kept, covering current and rollback revisions."
  type        = number
  default     = 50

  validation {
    condition     = var.artifact_keep_recent_versions >= 10
    error_message = "artifact_keep_recent_versions must keep at least 10 images to protect rollbacks."
  }
}

variable "artifact_delete_older_than_days" {
  description = "Delete images older than this many days unless kept by the recent-versions policy."
  type        = number
  default     = 180

  validation {
    condition     = var.artifact_delete_older_than_days >= 30
    error_message = "artifact_delete_older_than_days must be at least 30."
  }
}

variable "server_error_alert_threshold" {
  description = "Production 5xx responses tolerated within 5 minutes before alerting."
  type        = number
  default     = 5
}

variable "content_failure_alert_threshold" {
  description = "Production content failures (failed content refreshes and snapshot fallbacks) tolerated within 5 minutes before alerting."
  type        = number
  default     = 10
}

variable "startup_latency_alert_ms" {
  description = "Production container startup p95, in milliseconds, above which a 15-minute window alerts. The Go site starts in about 230 ms (plan Phase 8)."
  type        = number
  default     = 700
}

variable "firestore_read_alert_per_hour" {
  description = "Production Firestore document reads tolerated in one hour before alerting: about the plan's 40k/day bug threshold, as an hourly pace."
  type        = number
  default     = 2000
}
