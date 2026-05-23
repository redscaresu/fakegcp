# BROKEN: firewall rule points at a VPC network that is not declared
# in this configuration. Common real-world variant: the firewall was
# refactored to reference a VPC by string (instead of resource) during
# a module split, and the new module never created the network.
#
# ── Why standard tooling does not catch this ────────────────────────────
#
#   terraform validate  ✓ passes — `network` is typed string
#   terraform plan      ✓ passes — Terraform cannot verify the VPC
#                                  exists in the project
#
# ── What fakegcp catches ────────────────────────────────────────────────
#
#   $ terraform apply
#   Error: googleapi: Error 404: The resource was not found
#
#   fakegcp's CreateFirewall handler resolves `network` to a row in the
#   compute_networks repo (both self-link and relative-path forms are
#   supported). A missing parent surfaces as models.ErrNotFound → 404.
#   See handlers/fk_violation_test.go::TestFirewallFKViolationByRelativePath.
resource "google_compute_firewall" "orphan" {
  name    = "orphan-allow-ssh"
  network = "projects/fake-project/global/networks/does-not-exist"

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  source_ranges = ["0.0.0.0/0"]
}
