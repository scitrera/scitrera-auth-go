# Source provenance

Scitrera Auth was initially extracted on 2026-09-09.
Module/repository: `github.com/scitrera/scitrera-auth-go`.
The standalone source was exported without parent Git history. The current
product version is defined in `versions.yaml`.

## First-party inputs

Scitrera application monorepo input revision:
`505af26d7d44cd5aedcf05a50df6d9689ea11aed`.
The auth-go and frontend-superadmin input directories were clean at extraction.
Other unrelated monorepo edits were preserved and not included.

Allowlisted input paths:

- `auth-go/cmd`, `auth-go/internal`, `auth-go/go.mod`, `auth-go/go.sum`.
- `frontend-superadmin/src/components/ui/{button,input,card}.tsx` and `src/index.css`;
  auth form behavior and small utility conventions were adapted into a new
  auth-only client, rather than exporting the platform stores/API catalog.
- Reviewed auth subset of `backend/scitrera_app_server/mt/mt.sql`; its original
  table-only DDL is retained as a legacy compatibility test fixture.
- Scitrera's MemoryLayer Storage AGPL license and CLA text as local templates,
  with this project's name/scope and an independent CLA acceptance requirement.

No customer fixture records, concrete tenant IDs, deploy configurations, Python
backend, superadmin build graph, training data, credentials, certificates, logs,
or private Git history were copied. Existing auth tests were re-authored to use
reserved example domains and synthetic UUIDs. The default public login branding
no longer loads a remote Scitrera asset.

## Public dependency inputs

Aether server, API and Go SDK use the published **v0.2.4** module releases,
including shared browser-session inventory and revocation. The module tags
`server/v0.2.4`, `api/v0.2.4` and `sdk/go/v0.2.4` point to commit
`89e1f93857229716e570150aa776b392cf397f17` in the
[Aether v0.2.4 release](https://github.com/scitrera/aether/releases/tag/v0.2.4).
These dependencies were resolved from the public Go proxy and checksum database
with `GOWORK=off`; the standalone build has no local replacements.
Aether remains Apache-2.0.

| Module | Public checksum |
|---|---|
| `github.com/scitrera/aether/server v0.2.4` | `h1:cOJI9w/tL3UTlOH5HAWqLQVZdk8tY1UD0iZToTN7H5A=` |
| `github.com/scitrera/aether/api v0.2.4` | `h1:sJyVyTE9jj7ZEYmNtV/X+QYf0LpVUdcEbhInl+vi5mU=` |
| `github.com/scitrera/aether/sdk/go v0.2.4` | `h1:VDm88/6jZGUMym0QbBUB8acgMsioEyhVy6g4v7rPZQg=` |

`migrations/001_proxy.sql` adapts selected table definitions from Aether migrations
003, 007, 012 and 028, under Apache-2.0, in a dedicated schema. It omits the
upstream broad development ACL seeds and unrelated platform tables.

Go **1.25.14** is the requested project toolchain and is pinned in go.mod, CI and
the Dockerfile. An initial baseline trial used locally available Go 1.26.8 before
the toolchain preference was supplied; final verification uses 1.25.14.
Node 24.13.0 and all frontend packages are locked. Docker base images and Compose
PostgreSQL are pinned by digest. Third-party licenses, including the UI/font
dependencies, are inventoried in THIRD_PARTY_NOTICES.md.

## Export identity

`scripts/source.mjs` enumerates explicit source roots, excludes runtime files,
node_modules, generated bundles, Git data and private environments, and rejects
symlinks. It hashes sorted filenames and contents into `.source-revision`, then
creates a normalized source tarball embedded alongside the dashboard. Binary
`version` output includes this source digest. It identifies the source included
in each build separately from the historical input revision above.
