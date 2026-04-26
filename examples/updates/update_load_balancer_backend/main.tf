# Update flow for google_compute_backend_service: change description.
# Backend service updates use PUT (not PATCH) on the Compute API; the
# alias is wired through fakegcp/handlers/handlers.go.
#
#   terraform apply -var-file=v1.tfvars -auto-approve
#   terraform apply -var-file=v2.tfvars -auto-approve
#   terraform destroy -var-file=v2.tfvars -auto-approve
variable "backend_description" {
  type = string
}

resource "google_compute_health_check" "hc" {
  name = "lb-hc"

  http_health_check {
    port         = 80
    request_path = "/"
  }
}

resource "google_compute_backend_service" "be" {
  name          = "lb-be"
  description   = var.backend_description
  protocol      = "HTTP"
  health_checks = [google_compute_health_check.hc.id]
}
