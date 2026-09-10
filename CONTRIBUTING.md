# Contributing

Scitrera LLC is the sole Project Owner. Before merging a contribution, its
contributor must accept this project's [Scitrera Auth CLA](CLA.md) through a
process designated by Scitrera LLC. Acceptance for another project does not cover
this repository. No electronic CLA service is configured yet; coordinate with
[open-source-team@scitrera.com](mailto:open-source-team@scitrera.com).

Contributors retain ownership of their contributions. First-party contributions
use AGPL-3.0-only; preserve upstream Apache and other licenses and attribution.
Identify all third-party material and do not submit customer records, credentials,
private history, logs or training material.

Use Go 1.25.14, run `gofmt`, and run `npm run format` inside web/ for frontend
source. Run `make build` and `make check`, plus the PostgreSQL/browser scenarios
in docs/testing.md when changing authorization, persistence or UI behavior.
Update the lockfiles, notices and corresponding source archive with dependencies.

Set release versions and shared CI configuration in `versions.yaml`, then run
`make versions ci`. Use scitrera-repo-tools 0.1.30 as documented in the README.
Generated workflows must not be edited by hand; `make check-metadata` checks
their drift along with Go/npm versions, the npm lockfile and container tags.
