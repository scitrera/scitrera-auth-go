# Release preparation

`versions.yaml` is the single source of truth for the product version, Go
toolchain and shared CI settings.
Go remains 1.25.14. Dependency upgrades must preserve the public Aether
authproxy API and refresh notices and tests.

Use the public scitrera-repo-tools 0.1.30 package; no sibling checkout is required.
After changing the version or CI settings:

```sh
make versions ci
make build check
make artifacts container
```

`make versions` runs repo-tools `sync-versions` for the Go command and private
dashboard manifest, then synchronizes the npm lockfile and Docker/Compose version
mirrors with a project-specific adapter. Those mirrors let Docker-only users
build without Python tooling. `make ci` runs `generate-ci-gha --force` and prepares
the reusable container workflow using repo-tools' Docker generator. That small
adapter keeps the generated jobs intact and makes `release.yml` own their trigger
and ordering. It runs in uv's pinned repo-tools environment; installations using
pip can set `REPO_TOOLS_PYTHON=python3` instead.
`make check-metadata` checks all declarations, mirrors and generated workflows
without rewriting them. Edit the canonical YAML rather than a generated mirror.
`make build` and `make source` also validate the newly generated source archive,
requiring it to exist before compilation or packaging can continue.

Required gates before publication:

1. Run the Go race suite with PostgreSQL, vet, frontend typecheck/build/unit tests,
   and the real browser workflows in testing.md.
2. Build from the standalone root and from the exported corresponding-source
   archive. Run a container smoke test with a disposable migrated database.
3. Review the source allowlist, archive, image and UI bundle for confidential
   inputs; retain Apache/upstream license exceptions and notices.
4. Record source digest, binary/image digests and exact commands/results.
5. Commit the synchronized version and validated source before pushing its
   matching version tag. A tag push starts the release workflow below.

Release artifacts: source tarball, Linux amd64/arm64 `scitrera-auth-proxy`
binaries, SHA256SUMS, and a versioned OCI image. The Docker build embeds a source
archive and stamps the binary with its content digest. Supply `--build-arg
REVISION=<reviewed-source-revision>` to label the image, or use the release Git
commit in a publishing workflow while retaining the binary's source digest.
Runtime license texts are installed in `/usr/share/doc/scitrera-auth/`.

repo-tools generates `version-check.yml`, `test-go.yml` (PostgreSQL, vet,
uncached race tests and govulncheck) and `test-npm.yml` (typecheck, production build and unit
tests). The project-specific `ci.yml` checks metadata and the full embedded
dashboard build, release-gate tests, real browser workflows and the standalone
container.

`.github/workflows/release.yml` runs on pushed `v*.*.*` tags and manual dispatch.
It checks that a tag matches the product version in `versions.yaml`, runs all
three test workflows, and builds the embedded dashboard, Linux binaries and
corresponding-source archive. The artifacts and `SHA256SUMS` are uploaded as
`scitrera-auth-artifacts`.

Matching tag pushes then call `publish-container.yml`, generated from the
`docker` and `ci.docker` blocks in `versions.yaml`. It builds on native
`ubuntu-latest` (linux/amd64) and `ubuntu-24.04-arm` (linux/arm64) runners. Each
runner pushes its image by digest to `ghcr.io/scitrera/scitrera-auth-go`; a merge
job combines both digests into a multi-architecture manifest. No QEMU is used.
repo-tools supplies registry login, Docker metadata, per-platform caches, and
manifest merging. The jobs use `GITHUB_TOKEN` with `packages: write`.

For `v0.1.2`, the image receives `0.1.2`, `0.1`, `latest`, and a commit SHA tag
through repo-tools' standard Docker metadata rules. `VERSION` comes from
`versions.yaml`; `REVISION` and the OCI revision label identify the Git commit.
The executable retains its embedded corresponding-source content digest.
Container publication waits for tag validation, all three test workflows and
artifact preparation. The GitHub Release waits for the container manifest.

For a tag push, `ci.github_release: true` enables the final GitHub Release job.
That job verifies the artifact checksums, creates a release with generated notes,
and attaches both binaries, the source archive and checksums. Prerelease versions
are marked as prereleases. Setting the flag to `false` keeps artifact preparation
and GHCR publication but skips the GitHub Release. Manual dispatch always prepares artifacts only,
including when dispatched against a tag.

For version 0.1.2, push the committed source and its tag:

```sh
git push origin main
git tag v0.1.2
git push origin v0.1.2
```

Packaging remains project-specific because the generated Go binary workflow does
not prepare the embedded dashboard/source archive. The release workflow reads
the publication flag through scitrera-repo-tools' configuration loader; generated
`publish-go.yml` is excluded to keep a single GitHub Release publisher. The
standalone `build-docker.yml` is also excluded: its generated jobs are exposed
through the checked `publish-container.yml` adapter so they cannot publish ahead
of the release checks. Only the final GitHub Release job has `contents: write`;
the container publisher has `packages: write`. The workflow does not change
repository/package visibility or deploy the service.

Deferred scope: online-presence/access-log dashboards; general secret/global OAuth client
editing; platform provisioning and MemoryLayer user or
license synchronization; fleet controls; Helm publication; hard tenant deletion;
new password/MFA systems. Existing Aether machine credential/ACL provisioning is
an external integration, not a new operator dashboard feature.
