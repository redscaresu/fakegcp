terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 5.0"
    }
  }
}

# Point all endpoints at fakegcp.
# Start fakegcp: fakegcp --port 8080
provider "google" {
  project      = "fake-project"
  region       = "us-central1"
  zone         = "us-central1-a"
  access_token = "fake-token"

  # Disable batching so each resource gets its own request
  batching {
    send_after = "0s"
  }

  compute_custom_endpoint           = "http://localhost:8080/compute/v1/"
  container_custom_endpoint         = "http://localhost:8080/"
  cloud_sql_custom_endpoint         = "http://localhost:8080/sql/v1beta4/"
  iam_custom_endpoint               = "http://localhost:8080/"
  storage_custom_endpoint           = "http://localhost:8080/storage/v1/"
}
