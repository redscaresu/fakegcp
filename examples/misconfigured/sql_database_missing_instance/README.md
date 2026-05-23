# sql_database_missing_instance

A `google_sql_database` whose parent `google_sql_database_instance`
was never declared in the configuration.

## What's broken

```hcl
instance = "ghost-sql-instance"   # no instance resource declared
```

Cloud SQL databases are namespaced under an instance; the URL path on
create is `/sql/v1beta4/projects/{p}/instances/{instance}/databases`.
With no matching instance, fakegcp's FK lookup fails.

## Why standard tooling does not catch this

| Tool                 | Result | Reason                                           |
|----------------------|--------|--------------------------------------------------|
| `terraform validate` | passes | `instance` is typed string                       |
| `terraform plan`     | passes | Terraform cannot verify the instance exists      |

## What fakegcp catches

```
$ terraform apply
Error: googleapi: Error 404: The resource was not found
```

**Handler:** `handlers/sql.go::CreateSQLDatabase` → repo FK lookup on
`sql_instances`. The `sql_databases` table has a FK on `instance_name`
that rejects inserts referencing a missing parent.

**Regression coverage:** `handlers/handlers_test.go::TestSQLDatabaseFKViolation`.
