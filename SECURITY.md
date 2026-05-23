# Security Policy

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.
Instead, report privately via:

- GitHub's [private vulnerability reporting](https://github.com/redscaresu/fakegcp/security/advisories/new) (preferred).
- Email: `ukashouri@gmail.com` with subject prefix `[security] fakegcp:`.

Include: description, impact, steps to reproduce, affected commit, any mitigations you've identified.

## What to expect

- Acknowledgement within 5 working days.
- Assessment within 14 days of acknowledgement.
- Coordinated disclosure with credit unless you decline.

## Scope

In scope:
- This repository (`redscaresu/fakegcp`).
- Vulnerabilities in the SQLite-backed mock that would let a crafted HTTP request execute arbitrary code, exfiltrate filesystem contents outside the working directory, or otherwise escape the intended mock surface.

Out of scope:
- Issues in dependencies (`go-chi`, `modernc.org/sqlite`, etc.) — please report upstream.
- "Fakegcp accepts an invalid GCP request that real GCP would reject" — that's a *fidelity gap*, not a vulnerability; file a regular issue with the `fidelity` label.
