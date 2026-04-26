# Update flow for google_cloud_run_v2_service: change labels.
#
#   terraform apply -var-file=v1.tfvars -auto-approve
#   terraform apply -var-file=v2.tfvars -auto-approve
#   terraform destroy -var-file=v2.tfvars -auto-approve
variable "env_label" {
  type = string
}

resource "google_cloud_run_v2_service" "main" {
  name     = "api"
  location = "us-central1"

  labels = {
    env = var.env_label
  }

  template {
    containers {
      image = "us-docker.pkg.dev/cloudrun/container/hello"
    }
  }

  deletion_protection = false
}
