# BROKEN: compute instance's network_interface references a VPC network
# that was never declared in this configuration.
#
# A reviewer who only reads the .tf file sees a well-formed string for
# `network`; nothing in HCL flags it as bad. The instance create then
# hits fakegcp with a network reference that doesn't exist in the repo.
#
# ── Why standard tooling does not catch this ────────────────────────────
#
#   terraform validate  ✓ passes — `network` is typed string and the
#                                  value is a syntactically valid
#                                  resource path
#   terraform plan      ✓ passes — Terraform cannot dereference the
#                                  string to verify the network exists
#
# ── What fakegcp catches ────────────────────────────────────────────────
#
#   $ terraform apply
#   Error: googleapi: Error 404: The resource was not found
#
#   fakegcp's CreateInstance handler runs
#   validateInstanceNetworkInterfaces, which resolves every
#   network_interface[].network to an existing VPC in the same project.
#   A missing parent surfaces as models.ErrNotFound → 404 (see
#   handlers/fk_violation_test.go::TestInstanceFKViolationMissingNetwork).
resource "google_compute_instance" "orphan" {
  name         = "orphan-vm"
  machine_type = "e2-medium"
  zone         = "us-central1-a"

  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-11"
    }
  }

  network_interface {
    network = "projects/fake-project/global/networks/does-not-exist"
  }
}
