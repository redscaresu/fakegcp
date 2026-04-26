terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 5.0"
    }
  }
}

provider "google" {
  project      = "fake-project"
  region       = "us-central1"
  access_token = "fake-token"

  batching {
    send_after = "0s"
  }

  iam_custom_endpoint                    = "http://localhost:8080/v1/"
  cloud_resource_manager_custom_endpoint = "http://localhost:8080/v1/"
}
