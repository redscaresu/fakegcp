terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 5.0"
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

  cloud_run_v2_custom_endpoint = "http://localhost:8080/v2/"
}
