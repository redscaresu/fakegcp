# Secret Manager secret + secret version. Tests the secret -> version
# FK chain covered by fakegcp's TestSecretCRUD,
# TestSecretVersionCRUD, TestSecretDeleteWithVersions handler tests.

resource "google_secret_manager_secret" "main" {
  secret_id = "test-secret"

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "main" {
  secret      = google_secret_manager_secret.main.id
  secret_data = "fakegcp-test-payload"
}
