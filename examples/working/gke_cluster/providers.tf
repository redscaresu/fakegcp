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
  zone         = "us-central1-a"
  access_token = "fake-token"

  batching {
    send_after = "0s"
  }

  compute_custom_endpoint   = "http://localhost:8080/compute/v1/"
  container_custom_endpoint = "http://localhost:8080/"
}
