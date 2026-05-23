terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      # Pinned to ~> 5.0: provider-google v6+ split iam_custom_endpoint
      # away from the iam.admin.v1 API path used by google_service_account,
      # so SA create/read/delete hit real iam.googleapis.com with the
      # fake-token and 401. Stay on v5.x until fakegcp grows
      # iam_admin_v1_custom_endpoint support (BACKLOG M46).
      version = "~> 5.0"
    }
  }
}

# Start fakegcp: fakegcp --port 8080
provider "google" {
  project      = "fake-project"
  region       = "us-central1"
  access_token = "fake-token"

  batching {
    send_after = "0s"
  }

  # Endpoints: provider-google PREPENDS the API path (/v1/projects/...)
  # before hitting our endpoint, so the configured URL must NOT include
  # the trailing /v1/ for these particular services. M41/M46 closeout.
  iam_custom_endpoint                = "http://localhost:8080/"
  cloud_resource_manager_custom_endpoint = "http://localhost:8080/"
}
