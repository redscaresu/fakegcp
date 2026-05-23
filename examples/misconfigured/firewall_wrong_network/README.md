# firewall_wrong_network

A `google_compute_firewall` rule that targets a VPC network which
isn't declared in the configuration.

## What's broken

```hcl
network = "projects/fake-project/global/networks/does-not-exist"
```

The path string parses fine but resolves to no row in fakegcp's
compute_networks repo.

## Why standard tooling does not catch this

| Tool                 | Result | Reason                                       |
|----------------------|--------|----------------------------------------------|
| `terraform validate` | passes | `network` is typed string                    |
| `terraform plan`     | passes | Cannot verify the VPC exists in the project  |

## What fakegcp catches

```
$ terraform apply
Error: googleapi: Error 404: The resource was not found
```

**Handler:** `handlers/compute.go::CreateFirewall` → repo FK lookup on
the parent VPC. Both self-link and
`projects/<p>/global/networks/<n>` reference forms are validated.

**Regression coverage:**
`handlers/fk_violation_test.go::TestFirewallFKViolationByRelativePath`.
