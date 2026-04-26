# BROKEN: record set is created in a managed zone that doesn't exist.
#
# Cloud DNS record sets are addressed under their parent managedZone.
# The Terraform `managed_zone` attribute is a string identifier — when
# it points at a zone that hasn't been created, the apply still parses
# and plans cleanly.
#
# ── Why standard tooling does not catch this ────────────────────────────
#
#   terraform validate  ✓ passes — `managed_zone` is typed string
#   terraform plan      ✓ passes — Terraform cannot verify the parent
#                                  managedZone actually exists
#
# ── What fakegcp catches ────────────────────────────────────────────────
#
#   $ terraform apply
#   Error: googleapi: Error 404: managed zone "missing-zone" not found
#
#   fakegcp's dns_record_sets table joins to dns_managed_zones; the
#   create rejects because no parent zone row exists.
resource "google_dns_record_set" "broken" {
  name         = "host.example.invalid."
  managed_zone = "missing-zone"
  type         = "A"
  ttl          = 300
  rrdatas      = ["192.0.2.10"]
}
