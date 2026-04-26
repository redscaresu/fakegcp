# Update flow for google_dns_record_set: change TTL.
# Cloud DNS implements record-set updates via the v1 changes API
# (delete-then-add inside one transactional change), which fakegcp
# accepts atomically.
#
#   terraform apply -var-file=v1.tfvars -auto-approve
#   terraform apply -var-file=v2.tfvars -auto-approve
#   terraform destroy -var-file=v2.tfvars -auto-approve
variable "record_ttl" {
  type = number
}

resource "google_dns_managed_zone" "main" {
  name        = "test-zone"
  dns_name    = "test.example.invalid."
  description = "fakegcp e2e zone"
}

resource "google_dns_record_set" "main" {
  name         = "host.${google_dns_managed_zone.main.dns_name}"
  managed_zone = google_dns_managed_zone.main.name
  type         = "A"
  ttl          = var.record_ttl
  rrdatas      = ["192.0.2.10"]
}
