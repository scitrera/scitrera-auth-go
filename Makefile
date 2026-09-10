GO ?= go
PYTHON ?= python3
REPO_TOOLS ?= repo-tools
export GOWORK := off
export GOTOOLCHAIN := local
# Resolved only by targets that need it, so Go-only tests need no Python tools.
VERSION = $(shell $(REPO_TOOLS) sync-versions --print-version scitrera-auth-go)

.PHONY: versions ci check-metadata build web test check container source artifacts
versions:
	$(REPO_TOOLS) sync-versions
	$(PYTHON) scripts/sync-build-metadata.py "$(VERSION)"
ci:
	$(REPO_TOOLS) generate-ci-gha --force
check-metadata:
	$(REPO_TOOLS) sync-versions --check
	$(REPO_TOOLS) generate-ci-gha --check
	$(PYTHON) scripts/sync-build-metadata.py --check "$(VERSION)"
	$(PYTHON) scripts/check-release.py
web:
	cd web && npm ci && npm run build
build: check-metadata web
	mkdir -p dist
	$(GO) build -buildvcs=false -trimpath -ldflags "-s -w -X main.revision=$$(cat .source-revision)" -o dist/scitrera-auth-proxy ./cmd/scitrera-auth-proxy
test:
	$(GO) test -race ./...
	cd web && npm test
check: check-metadata test
	$(GO) vet ./...
	cd web && npm run typecheck
container: check-metadata source
	docker build --build-arg VERSION=$(VERSION) --build-arg REVISION=$$(cat .source-revision) -t scitrera-auth-go:$(VERSION) .
source:
	node scripts/source.mjs
artifacts: build
	VERSION="$(VERSION)" GO="$(GO)" sh scripts/artifacts.sh
