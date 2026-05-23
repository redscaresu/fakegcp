# BROKEN: subnetwork's `network` attribute points at a VPC that does
# not exist in this project. A typical real-world variant is a
# misspelled VPC name copied from another environment.
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
#   fakegcp's CreateSubnetwork handler resolves `network` (both
#   self-link and `projects/<p>/global/networks/<n>` forms) to a row in
#   the compute_networks repo. A missing parent surfaces as
#   models.ErrNotFound → 404. See
#   handlers/fk_violation_test.go::TestSubnetworkFKViolationByRegionalPath.
resource "google_compute_subnetwork" "orphan" {
  name          = "orphan-subnet"
  ip_cidr_range = "10.0.0.0/24"
  region        = "us-central1"
  network       = "projects/fake-project/global/networks/does-not-exist"
}
