# DELIBERATELY MISCONFIGURED: subnetwork references a non-existent network.
# Expected: fakegcp returns 404 on subnetwork create (FK violation).
# This catches a bug that terraform validate and terraform plan cannot detect.

resource "google_compute_subnetwork" "orphan" {
  name          = "orphan-subnet"
  ip_cidr_range = "10.0.0.0/24"
  region        = "us-central1"
  network       = "projects/fake-project/global/networks/does-not-exist"
}
