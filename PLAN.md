# fakegcp — Stateful GCP API Mock

A stateful mock of the Google Cloud Platform API for offline Terraform testing. Think LocalStack, but for GCP. Single Go binary, SQLite state, path-based routing. Modeled after [mockway](https://github.com/redscaresu/mockway).

## Why

`terraform validate` and `terraform plan` cannot catch:
- FK references (creating instance in non-existent network)
- Dependency ordering (deleting network with active instances)
- Response shape mismatches (drift on second apply)

fakegcp enforces these at the HTTP level with SQLite FK constraints.

## Architecture (mirrors mockway)

```
fakegcp/
├── cmd/fakegcp/main.go              # Entrypoint: CLI flags, server setup, DI wiring
├── handlers/
│   ├── handlers.go                  # Application struct, RegisterRoutes(), auth, error helpers
│   ├── compute.go                   # Instances, disks, addresses
│   ├── network.go                   # Networks, subnetworks, firewalls
│   ├── container.go                 # GKE clusters, node pools
│   ├── sql.go                       # Cloud SQL instances, databases, users
│   ├── iam.go                       # Service accounts, keys
│   ├── storage.go                   # Buckets
│   ├── operations.go                # Operation polling (always DONE)
│   ├── admin.go                     # /mock/reset, /mock/state, /mock/snapshot
│   ├── unimplemented.go             # 501 catch-all
│   └── handlers_test.go            # Integration tests
├── repository/repository.go         # SQLite schema, CRUD, FK enforcement
├── models/models.go                 # ErrNotFound, ErrConflict, ErrAlreadyExists
├── testutil/testutil.go             # NewTestServer, HTTP helpers
├── examples/
│   ├── working/                     # Verified idempotent configs
│   └── misconfigured/               # Deliberate FK violations
├── AGENTS.md                        # Dev guide (generated after build)
├── go.mod
├── Makefile
└── Dockerfile
```

## GCP API Conventions (differs from Scaleway)

### Authentication
- Header: `Authorization: Bearer <token>` (any non-empty value accepted)
- Missing/empty → 401 `{"error": {"code": 401, "message": "Request is missing required authentication credential."}}`

### URL Structure
```
/compute/v1/projects/{project}/zones/{zone}/instances/{name}
/compute/v1/projects/{project}/regions/{region}/subnetworks/{name}
/compute/v1/projects/{project}/global/networks/{name}
/compute/v1/projects/{project}/zones/{zone}/operations/{name}
/v1/projects/{project}/locations/{location}/clusters/{name}
/sql/v1beta4/projects/{project}/instances/{name}
/v1/projects/{project}/serviceAccounts/{email}
/storage/v1/b/{bucket}
```

### Resource Identity
- GCP uses **name** (user-provided string) as identifier, NOT auto-generated UUIDs
- Resources are uniquely identified by: `project + scope(zone/region/global) + name`
- `id` field is a numeric string (auto-generated, for display only)
- `selfLink` is computed: `http://{host}/{api_path}/projects/{project}/.../resources/{name}`

### Response Format
```json
// Single resource
{"kind": "compute#instance", "id": "1234567890", "name": "my-vm", "selfLink": "...", ...}

// List
{"kind": "compute#instanceList", "items": [...], "selfLink": "..."}
// Note: empty list omits "items" key entirely (GCP convention)

// Error
{"error": {"code": 404, "message": "The resource ... was not found", "errors": [{"message": "...", "domain": "global", "reason": "notFound"}]}}
```

### Operations Model
GCP mutations return an Operation object. The Terraform provider polls until `status: "DONE"`.

**fakegcp strategy**: Return operations as immediately DONE with embedded target resource link. This avoids polling complexity while satisfying the provider.

```json
{
  "kind": "compute#operation",
  "id": "op-123",
  "name": "operation-uuid",
  "status": "DONE",
  "targetLink": "http://localhost:8080/compute/v1/projects/my-proj/zones/us-central1-a/instances/my-vm",
  "operationType": "insert",
  "progress": 100,
  "selfLink": "..."
}
```

## Initial Scope (Phase 1)

### Compute Engine — `/compute/v1/projects/{project}/...`

| Resource | Table | FK | Terraform Resource |
|----------|-------|----|--------------------|
| Instance | `compute_instances` | network, subnetwork, disk | `google_compute_instance` |
| Network | `compute_networks` | — | `google_compute_network` |
| Subnetwork | `compute_subnetworks` | network | `google_compute_subnetwork` |
| Firewall | `compute_firewalls` | network | `google_compute_firewall` |
| Disk | `compute_disks` | — | `google_compute_disk` |
| Address | `compute_addresses` | — | `google_compute_address` |

### Container (GKE) — `/v1/projects/{project}/locations/{location}/...`

| Resource | Table | FK | Terraform Resource |
|----------|-------|----|--------------------|
| Cluster | `container_clusters` | network, subnetwork | `google_container_cluster` |
| Node Pool | `container_node_pools` | cluster | `google_container_node_pool` |

### Cloud SQL — `/sql/v1beta4/projects/{project}/...`

| Resource | Table | FK | Terraform Resource |
|----------|-------|----|--------------------|
| Instance | `sql_instances` | network (optional) | `google_sql_database_instance` |
| Database | `sql_databases` | instance | `google_sql_database` |
| User | `sql_users` | instance | `google_sql_user` |

### IAM — `/v1/projects/{project}/...`

| Resource | Table | FK | Terraform Resource |
|----------|-------|----|--------------------|
| Service Account | `iam_service_accounts` | — | `google_service_account` |
| SA Key | `iam_sa_keys` | service_account | `google_service_account_key` |

### Storage — `/storage/v1/...`

| Resource | Table | FK | Terraform Resource |
|----------|-------|----|--------------------|
| Bucket | `storage_buckets` | — | `google_storage_bucket` |

## SQLite Schema

```sql
PRAGMA foreign_keys = ON;

-- Compute
CREATE TABLE compute_networks (
    name TEXT NOT NULL,
    project TEXT NOT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY (project, name)
);

CREATE TABLE compute_subnetworks (
    name TEXT NOT NULL,
    project TEXT NOT NULL,
    region TEXT NOT NULL,
    network_name TEXT NOT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY (project, region, name),
    FOREIGN KEY (project, network_name) REFERENCES compute_networks(project, name)
);

CREATE TABLE compute_firewalls (
    name TEXT NOT NULL,
    project TEXT NOT NULL,
    network_name TEXT NOT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY (project, name),
    FOREIGN KEY (project, network_name) REFERENCES compute_networks(project, name)
);

CREATE TABLE compute_disks (
    name TEXT NOT NULL,
    project TEXT NOT NULL,
    zone TEXT NOT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY (project, zone, name)
);

CREATE TABLE compute_instances (
    name TEXT NOT NULL,
    project TEXT NOT NULL,
    zone TEXT NOT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY (project, zone, name)
);

CREATE TABLE compute_addresses (
    name TEXT NOT NULL,
    project TEXT NOT NULL,
    region TEXT NOT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY (project, region, name)
);

-- Operations (ephemeral, auto-cleaned)
CREATE TABLE operations (
    name TEXT NOT NULL,
    project TEXT NOT NULL,
    zone TEXT DEFAULT NULL,
    region TEXT DEFAULT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY (project, name)
);

-- Container (GKE)
CREATE TABLE container_clusters (
    name TEXT NOT NULL,
    project TEXT NOT NULL,
    location TEXT NOT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY (project, location, name)
);

CREATE TABLE container_node_pools (
    name TEXT NOT NULL,
    project TEXT NOT NULL,
    location TEXT NOT NULL,
    cluster_name TEXT NOT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY (project, location, cluster_name, name),
    FOREIGN KEY (project, location, cluster_name) REFERENCES container_clusters(project, location, name) ON DELETE CASCADE
);

-- Cloud SQL
CREATE TABLE sql_instances (
    name TEXT NOT NULL,
    project TEXT NOT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY (project, name)
);

CREATE TABLE sql_databases (
    name TEXT NOT NULL,
    project TEXT NOT NULL,
    instance_name TEXT NOT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY (project, instance_name, name),
    FOREIGN KEY (project, instance_name) REFERENCES sql_instances(project, name) ON DELETE CASCADE
);

CREATE TABLE sql_users (
    name TEXT NOT NULL,
    project TEXT NOT NULL,
    instance_name TEXT NOT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY (project, instance_name, name),
    FOREIGN KEY (project, instance_name) REFERENCES sql_instances(project, name) ON DELETE CASCADE
);

-- IAM
CREATE TABLE iam_service_accounts (
    unique_id TEXT NOT NULL,
    project TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE,
    data TEXT NOT NULL,
    PRIMARY KEY (project, unique_id)
);

CREATE TABLE iam_sa_keys (
    name TEXT NOT NULL PRIMARY KEY,
    project TEXT NOT NULL,
    service_account_email TEXT NOT NULL,
    data TEXT NOT NULL,
    FOREIGN KEY (service_account_email) REFERENCES iam_service_accounts(email)
);

-- Storage
CREATE TABLE storage_buckets (
    name TEXT NOT NULL PRIMARY KEY,
    project TEXT NOT NULL,
    location TEXT NOT NULL,
    data TEXT NOT NULL
);
```

## Handler Patterns

### GCP Error Helper
```go
func writeGCPError(w http.ResponseWriter, code int, message, reason string) {
    writeJSON(w, code, map[string]any{
        "error": map[string]any{
            "code":    code,
            "message": message,
            "errors":  []map[string]any{{"message": message, "domain": "global", "reason": reason}},
        },
    })
}
```

### Auth Middleware
```go
func requireBearerToken(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        auth := r.Header.Get("Authorization")
        if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") == "" {
            writeGCPError(w, 401, "Request is missing required authentication credential.", "required")
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

### Operation Helper
```go
func (app *Application) newOperation(project, zone, region, targetLink, opType string) map[string]any {
    name := "operation-" + uuid.NewString()
    op := map[string]any{
        "kind":          "compute#operation",
        "id":            numericID(),
        "name":          name,
        "status":        "DONE",
        "targetLink":    targetLink,
        "operationType": opType,
        "progress":      100,
        "startTime":     nowRFC3339(),
        "endTime":       nowRFC3339(),
    }
    // Store for polling
    app.repo.StoreOperation(project, zone, region, name, op)
    return op
}
```

### Create Handler Pattern
```go
func (app *Application) CreateInstance(w http.ResponseWriter, r *http.Request) {
    project := chi.URLParam(r, "project")
    zone := chi.URLParam(r, "zone")

    body, err := decodeBody(r)
    if err != nil {
        writeGCPError(w, 400, "Invalid JSON", "invalid")
        return
    }

    name, _ := body["name"].(string)
    if name == "" {
        writeGCPError(w, 400, "Name is required", "required")
        return
    }

    // Server-generated fields
    body["id"] = numericID()
    body["selfLink"] = selfLink(r, project, "zones", zone, "instances", name)
    body["status"] = "RUNNING"
    body["creationTimestamp"] = nowRFC3339()
    body["kind"] = "compute#instance"
    body["zone"] = zoneSelfLink(r, project, zone)

    out, err := app.repo.CreateInstance(project, zone, name, body)
    if err != nil {
        writeCreateError(w, err) // ErrNotFound→404, ErrAlreadyExists→409
        return
    }

    op := app.newOperation(project, zone, "", selfLink(r, project, "zones", zone, "instances", name), "insert")
    writeJSON(w, http.StatusOK, op)
}
```

### selfLink Helper
```go
func selfLink(r *http.Request, parts ...string) string {
    scheme := "http"
    return fmt.Sprintf("%s://%s/%s", scheme, r.Host, strings.Join(parts, "/"))
}
```

## CLI Flags
```
fakegcp --port 8080                    # Default: 8080, in-memory DB
fakegcp --port 8080 --db ./fakegcp.db  # File-backed persistence
fakegcp --echo --port 8080             # Echo mode for endpoint discovery
```

## Terraform Provider Configuration
```hcl
provider "google" {
  project     = "fake-project"
  region      = "us-central1"
  zone        = "us-central1-a"
  access_token = "fake-token"

  # Point all service endpoints at fakegcp
  batching {
    send_after = "0s"
  }
}
```

The google provider uses per-service endpoint overrides. Each must be set:
```hcl
provider "google" {
  ...
  compute_custom_endpoint                = "http://localhost:8080/compute/v1/"
  container_custom_endpoint              = "http://localhost:8080/"
  cloud_sql_custom_endpoint              = "http://localhost:8080/sql/v1beta4/"
  iam_custom_endpoint                    = "http://localhost:8080/v1/"
  storage_custom_endpoint                = "http://localhost:8080/storage/v1/"
  cloud_resource_manager_custom_endpoint = "http://localhost:8080/v1/"
  pubsub_custom_endpoint                 = "http://localhost:8080/v1/"
  dns_custom_endpoint                    = "http://localhost:8080/dns/v1/"
  cloud_run_v2_custom_endpoint           = "http://localhost:8080/v2/"
  secret_manager_custom_endpoint         = "http://localhost:8080/v1/"
}
```

## Route Registration

```go
func (app *Application) RegisterRoutes(r chi.Router) {
    // Admin (no auth)
    r.Post("/mock/reset", app.ResetState)
    r.Post("/mock/snapshot", app.SnapshotState)
    r.Post("/mock/restore", app.RestoreState)
    r.Get("/mock/state", app.FullState)
    r.Get("/mock/state/{service}", app.ServiceState)

    // GCP routes (auth required)
    r.Group(func(r chi.Router) {
        r.Use(requireBearerToken)

        // Compute
        r.Route("/compute/v1/projects/{project}", func(r chi.Router) {
            // Global
            r.Route("/global", func(r chi.Router) {
                r.Get("/networks", app.ListNetworks)
                r.Post("/networks", app.CreateNetwork)
                r.Get("/networks/{name}", app.GetNetwork)
                r.Delete("/networks/{name}", app.DeleteNetwork)
                r.Patch("/networks/{name}", app.UpdateNetwork)

                r.Get("/firewalls", app.ListFirewalls)
                r.Post("/firewalls", app.CreateFirewall)
                r.Get("/firewalls/{name}", app.GetFirewall)
                r.Delete("/firewalls/{name}", app.DeleteFirewall)
                r.Patch("/firewalls/{name}", app.UpdateFirewall)
            })

            // Zonal
            r.Route("/zones/{zone}", func(r chi.Router) {
                r.Get("/instances", app.ListInstances)
                r.Post("/instances", app.CreateInstance)
                r.Get("/instances/{name}", app.GetInstance)
                r.Delete("/instances/{name}", app.DeleteInstance)

                r.Get("/disks", app.ListDisks)
                r.Post("/disks", app.CreateDisk)
                r.Get("/disks/{name}", app.GetDisk)
                r.Delete("/disks/{name}", app.DeleteDisk)

                r.Get("/operations/{name}", app.GetOperation)
            })

            // Regional
            r.Route("/regions/{region}", func(r chi.Router) {
                r.Get("/subnetworks", app.ListSubnetworks)
                r.Post("/subnetworks", app.CreateSubnetwork)
                r.Get("/subnetworks/{name}", app.GetSubnetwork)
                r.Delete("/subnetworks/{name}", app.DeleteSubnetwork)
                r.Patch("/subnetworks/{name}", app.UpdateSubnetwork)

                r.Get("/addresses", app.ListAddresses)
                r.Post("/addresses", app.CreateAddress)
                r.Get("/addresses/{name}", app.GetAddress)
                r.Delete("/addresses/{name}", app.DeleteAddress)

                r.Get("/operations/{name}", app.GetOperation)
            })

            // Aggregated (Terraform uses these for data sources)
            r.Get("/aggregated/instances", app.AggregatedListInstances)
            r.Get("/aggregated/subnetworks", app.AggregatedListSubnetworks)
        })

        // Container (GKE)
        r.Route("/v1/projects/{project}/locations/{location}", func(r chi.Router) {
            r.Get("/clusters", app.ListClusters)
            r.Post("/clusters", app.CreateCluster)
            r.Get("/clusters/{name}", app.GetCluster)
            r.Delete("/clusters/{name}", app.DeleteCluster)
            r.Put("/clusters/{name}", app.UpdateCluster)

            r.Get("/clusters/{cluster}/nodePools", app.ListNodePools)
            r.Post("/clusters/{cluster}/nodePools", app.CreateNodePool)
            r.Get("/clusters/{cluster}/nodePools/{name}", app.GetNodePool)
            r.Delete("/clusters/{cluster}/nodePools/{name}", app.DeleteNodePool)
        })

        // Cloud SQL
        r.Route("/sql/v1beta4/projects/{project}", func(r chi.Router) {
            r.Get("/instances", app.ListSQLInstances)
            r.Post("/instances", app.CreateSQLInstance)
            r.Get("/instances/{name}", app.GetSQLInstance)
            r.Delete("/instances/{name}", app.DeleteSQLInstance)
            r.Patch("/instances/{name}", app.UpdateSQLInstance)

            r.Get("/instances/{instance}/databases", app.ListSQLDatabases)
            r.Post("/instances/{instance}/databases", app.CreateSQLDatabase)
            r.Get("/instances/{instance}/databases/{name}", app.GetSQLDatabase)
            r.Delete("/instances/{instance}/databases/{name}", app.DeleteSQLDatabase)

            r.Get("/instances/{instance}/users", app.ListSQLUsers)
            r.Post("/instances/{instance}/users", app.CreateSQLUser)
            r.Delete("/instances/{instance}/users", app.DeleteSQLUser)
            r.Put("/instances/{instance}/users", app.UpdateSQLUser)
        })

        // IAM
        r.Route("/v1/projects/{project}", func(r chi.Router) {
            r.Get("/serviceAccounts", app.ListServiceAccounts)
            r.Post("/serviceAccounts", app.CreateServiceAccount)
            r.Get("/serviceAccounts/{email}", app.GetServiceAccount)
            r.Delete("/serviceAccounts/{email}", app.DeleteServiceAccount)

            r.Post("/serviceAccounts/{email}/keys", app.CreateSAKey)
            r.Get("/serviceAccounts/{email}/keys", app.ListSAKeys)
            r.Get("/serviceAccounts/{email}/keys/{keyId}", app.GetSAKey)
            r.Delete("/serviceAccounts/{email}/keys/{keyId}", app.DeleteSAKey)
        })

        // Storage
        r.Route("/storage/v1", func(r chi.Router) {
            r.Get("/b", app.ListBuckets)
            r.Post("/b", app.CreateBucket)
            r.Get("/b/{bucket}", app.GetBucket)
            r.Delete("/b/{bucket}", app.DeleteBucket)
            r.Patch("/b/{bucket}", app.UpdateBucket)
        })
    })

    // Catch-all
    r.NotFound(app.Unimplemented)
    r.MethodNotAllowed(app.Unimplemented)
}
```

## Server-Generated Fields

| Resource | Fields |
|----------|--------|
| All | `id` (numeric string), `selfLink`, `creationTimestamp` (RFC3339) |
| Instance | `status: "RUNNING"`, `zone` (selfLink) |
| Network | `selfLink`, `autoCreateSubnetworks` (default true) |
| Subnetwork | `selfLink`, `gatewayAddress`, `fingerprint` |
| Firewall | `selfLink`, `direction` (default "INGRESS") |
| Disk | `status: "READY"`, `selfLink` |
| Address | `status: "RESERVED"`, `address` (random IP) |
| GKE Cluster | `status: "RUNNING"`, `endpoint` (fake IP), `masterAuth` |
| Node Pool | `status: "RUNNING"` |
| SQL Instance | `state: "RUNNABLE"`, `connectionName`, `ipAddresses` |
| Service Account | `uniqueId`, `email: "{name}@{project}.iam.gserviceaccount.com"` |
| SA Key | `privateKeyData` (base64 fake key) |
| Bucket | `timeCreated`, `updated`, `etag` |

## Build Order

### Step 1: Scaffold
- `go.mod`, `cmd/fakegcp/main.go`, `models/models.go`
- Dependencies: chi, uuid, sqlite, testify

### Step 2: Repository
- `repository/repository.go` — schema, migrate, CRUD helpers, all resource methods

### Step 3: Handlers
- `handlers/handlers.go` — Application, RegisterRoutes, auth, error helpers, selfLink
- `handlers/compute.go` — instances, disks, addresses
- `handlers/network.go` — networks, subnetworks, firewalls
- `handlers/operations.go` — operation store/get
- `handlers/container.go` — GKE clusters, node pools
- `handlers/sql.go` — Cloud SQL instances, databases, users
- `handlers/iam.go` — service accounts, keys
- `handlers/storage.go` — buckets
- `handlers/admin.go` — /mock/* endpoints
- `handlers/unimplemented.go` — 501 catch-all

### Step 4: Test Infrastructure
- `testutil/testutil.go` — NewTestServer, HTTP helpers
- `handlers/handlers_test.go` — integration tests

### Step 5: Examples
- `examples/working/basic_instance/` — instance + network + subnetwork
- `examples/working/gke_cluster/` — GKE with node pools
- `examples/working/cloud_sql/` — SQL instance + db + user
- `examples/misconfigured/instance_missing_network/` — FK violation

### Step 6: Docs
- `AGENTS.md` — dev guide
- `Makefile`, `Dockerfile`

## Verification
1. `go build ./...` — compiles
2. `go test ./...` — integration tests pass
3. `go test -tags provider_e2e ./e2e -v` — Terraform examples are idempotent
4. Double-apply test: `terraform apply` twice, second must be no-op

## Delegation Strategy (Claude + Codex)

**Claude (architect/orchestrate)**:
- Design schema, routes, handler signatures
- Wire DI, integration, error mapping
- Review codex output for correctness
- Write tests and examples

**Codex (code generation)**:
- Generate repository CRUD methods (bulk, repetitive)
- Generate handler implementations from patterns
- Generate test cases from handler patterns
