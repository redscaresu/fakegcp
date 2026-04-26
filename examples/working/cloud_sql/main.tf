# Cloud SQL instance with database and user.
# Tests: instance → database/user FK chain.

resource "google_sql_database_instance" "main" {
  name             = "test-sql-instance"
  database_version = "POSTGRES_15"
  region           = "us-central1"

  settings {
    tier = "db-f1-micro"
  }

  deletion_protection = false
}

resource "google_sql_database" "main" {
  name     = "test-db"
  instance = google_sql_database_instance.main.name
}

resource "google_sql_user" "main" {
  name     = "test-user"
  instance = google_sql_database_instance.main.name
  password = "changeme"
}
