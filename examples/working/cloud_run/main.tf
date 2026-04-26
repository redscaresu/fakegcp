# Cloud Run service (v2 API). Tests the service Create -> Get ->
# Update -> Delete cycle covered by fakegcp's TestCloudRunServiceCRUD
# handler test.

resource "google_cloud_run_v2_service" "main" {
  name     = "test-service"
  location = "us-central1"

  template {
    containers {
      image = "us-docker.pkg.dev/cloudrun/container/hello"

      env {
        name  = "GREETING"
        value = "hello-fakegcp"
      }
    }
  }

  deletion_protection = false
}
