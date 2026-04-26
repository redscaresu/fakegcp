# Cloud Storage bucket with uniform bucket-level access enabled.
# Maps to fakegcp's handlers/storage.go. The bucket name uses a
# project-prefix to mimic the global-uniqueness requirement of
# the real Cloud Storage namespace, even though fakegcp scopes
# everything per-project.
resource "google_storage_bucket" "assets" {
  name          = "fake-project-app-assets"
  location      = "us-central1"
  force_destroy = true

  uniform_bucket_level_access = true

  encryption {
    default_kms_key_name = "projects/fake-project/locations/us-central1/keyRings/r/cryptoKeys/k"
  }
}
