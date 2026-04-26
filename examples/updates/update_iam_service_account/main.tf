# Update flow for google_service_account: change display_name.
# IAM v1 PATCH wraps the new fields in a `serviceAccount` envelope —
# fakegcp's UpdateServiceAccount handler unwraps it before merging.
#
#   terraform apply -var-file=v1.tfvars -auto-approve
#   terraform apply -var-file=v2.tfvars -auto-approve
#   terraform destroy -var-file=v2.tfvars -auto-approve
variable "display_name" {
  type = string
}

resource "google_service_account" "ci" {
  account_id   = "ci-runner"
  display_name = var.display_name
}
