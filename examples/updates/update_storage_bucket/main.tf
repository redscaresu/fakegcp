# Update flow for google_storage_bucket: change labels.
#
#   terraform apply -var-file=v1.tfvars -auto-approve
#   terraform apply -var-file=v2.tfvars -auto-approve
#   terraform destroy -var-file=v2.tfvars -auto-approve
variable "env_label" {
  type = string
}

resource "google_storage_bucket" "assets" {
  name          = "fake-project-app-assets"
  location      = "us-central1"
  force_destroy = true

  uniform_bucket_level_access = true

  labels = {
    env = var.env_label
  }

  encryption {
    default_kms_key_name = "projects/fake-project/locations/us-central1/keyRings/r/cryptoKeys/k"
  }
}
