# BROKEN: Cloud SQL database targets an instance that is not declared
# in this configuration. Typical real-world variant: the database
# resource is copied from one stack into another but the corresponding
# `google_sql_database_instance` resource is left behind.
#
# ── Why standard tooling does not catch this ────────────────────────────
#
#   terraform validate  ✓ passes — `instance` is typed string
#   terraform plan      ✓ passes — Terraform cannot verify the instance
#                                  exists in the project
#
# ── What fakegcp catches ────────────────────────────────────────────────
#
#   $ terraform apply
#   Error: googleapi: Error 404: The resource was not found
#
#   The Cloud SQL database create path is keyed by instance name in the
#   URL. The sql_databases table has a FK on `instance_name`; insert
#   against a missing instance returns models.ErrNotFound → 404. The
#   handler is handlers/sql.go::CreateSQLDatabase; the existing
#   TestSQLDatabaseFKViolation case in handlers_test.go pins the
#   behaviour.
resource "google_sql_database" "orphan" {
  name     = "orphan-db"
  instance = "ghost-sql-instance"
}
