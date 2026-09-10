# Release preparation

`versions.yaml` is the single source of truth for the product version, Go
toolchain and shared CI settings. The candidate has not been publicly released.
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
build without Python tooling. `make ci` runs `generate-ci-gha --force`.
`make check-metadata` checks all declarations, mirrors and generated workflows
without rewriting them. Edit the canonical YAML rather than a generated mirror.

Required gates before publication:

1. Run the Go race suite with PostgreSQL, vet, frontend typecheck/build/unit tests,
   and the real browser workflows in testing.md.
2. Build from the standalone root and from the exported corresponding-source
   archive. Run a container smoke test with a disposable migrated database.
3. Review the source allowlist, archive, image and UI bundle for confidential
   inputs; retain Apache/upstream license exceptions and notices.
4. Record source digest, binary/image digests and exact commands/results.
5. Obtain publication review approval separately before creating a public repo,
   tagging, changing visibility, pushing a registry image or deploying.

Proposed artifacts: source tarball, Linux amd64/arm64 `scitrera-auth-proxy`
binaries, SHA256SUMS, and a versioned OCI image. The Docker build embeds a source
archive and stamps the binary with its content digest. Supply `--build-arg
REVISION=<reviewed-source-revision>` to label the image, or use the eventual Git
commit in a publishing workflow while retaining the binary's source digest.
Runtime license texts are installed in `/usr/share/doc/scitrera-auth/`.

repo-tools generates `version-check.yml`, `test-go.yml` (PostgreSQL, vet and
uncached race tests) and `test-npm.yml` (typecheck, production build and unit
tests). The project-specific `ci.yml` checks metadata and the full embedded
dashboard build, real browser workflows and the standalone container. The manual
`release-candidate.yml` runs all three test workflows before building and uploading
review artifacts. Its packaging remains project-specific because the generated
Go binary workflow does not yet prepare the embedded dashboard/source archive.
It has no registry push, public GitHub Release, tag or deployment permissions.
Remote CI execution and registry publishing must be recorded separately; local
container builds do not imply either occurred.

Deferred scope: session/access-log dashboards; general secret/global OAuth client
editing; platform provisioning and MemoryLayer user or
license synchronization; fleet controls; Helm publication; hard tenant deletion;
new password/MFA systems. Existing Aether machine credential/ACL provisioning is
an external integration, not a new operator dashboard feature.
