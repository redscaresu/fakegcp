# node_pool_missing_cluster

A `google_container_node_pool` that targets a GKE cluster which does
not exist in the project.

## What's broken

```hcl
cluster = "ghost-cluster"   # no google_container_cluster resource declared
```

The node pool create request is routed under the cluster's path
(`/projects/{p}/locations/{l}/clusters/{cluster}/nodePools`). When that
parent doesn't exist in fakegcp, the FK lookup fails.

## Why standard tooling does not catch this

| Tool                 | Result | Reason                                          |
|----------------------|--------|-------------------------------------------------|
| `terraform validate` | passes | `cluster` is typed string                       |
| `terraform plan`     | passes | Terraform cannot verify the cluster exists      |

## What fakegcp catches

```
$ terraform apply
Error: googleapi: Error 404: The resource was not found
```

**Handler:** `handlers/container.go::CreateNodePool` → repo FK lookup
on parent cluster (`container_clusters`).

**Regression coverage:**
`handlers/fk_violation_test.go::TestNodePoolFKViolationViaRelativePath`.
