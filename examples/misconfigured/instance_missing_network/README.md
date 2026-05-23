# instance_missing_network

A `google_compute_instance` whose `network_interface.network` points at
a VPC that was never declared in the configuration.

## What's broken

```hcl
network_interface {
  network = "projects/fake-project/global/networks/does-not-exist"
}
```

The network resource is **not declared**, so Terraform has nothing to
graph against and the apply hits fakegcp's create endpoint with a stale
self-link.

## Why standard tooling does not catch this

| Tool                 | Result | Reason                                         |
|----------------------|--------|------------------------------------------------|
| `terraform validate` | passes | `network` is typed string                      |
| `terraform plan`     | passes | Cannot dereference the string to verify        |

## What fakegcp catches

```
$ terraform apply
Error: googleapi: Error 404: The resource was not found
```

**Handler:** `handlers/compute.go::CreateInstance` →
`validateInstanceNetworkInterfaces` resolves each `network` value to a
row in the compute_networks repo. A missing parent surfaces as
`models.ErrNotFound` → HTTP 404 via `writeCreateError`.

**Regression coverage:**
`handlers/fk_violation_test.go::TestInstanceFKViolationMissingNetwork`.
