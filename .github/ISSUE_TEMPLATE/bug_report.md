---
name: Bug report
about: Mock behavior diverges from real GCP, or fakegcp crashes.
title: '[bug] '
labels: bug
assignees: ''
---

## Summary

<!-- One sentence describing what's wrong. -->

## Steps to reproduce

```bash
# example
./fakegcp --port 8080 &
curl -X POST 'http://localhost:8080/compute/v1/projects/test/global/networks' ...
```

## Expected behavior

<!-- What did real GCP return? Real-GCP raw HTTP response if you have one. -->

## Actual behavior

<!-- What did fakegcp return? Raw HTTP response. -->

## Environment

- fakegcp commit: <!-- `git rev-parse --short HEAD` -->
- OS / arch:
- Go version:
- terraform-provider-google version (if applicable):

## Type of issue

- [ ] Crash / panic in fakegcp
- [ ] Fidelity gap: fakegcp accepts a request that real GCP rejects
- [ ] Fidelity gap: fakegcp rejects a request that real GCP accepts
- [ ] Wrong response shape (fakegcp returns different fields than real GCP)
- [ ] FK enforcement bug
- [ ] Cascade delete bug
