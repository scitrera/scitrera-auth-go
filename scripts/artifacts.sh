#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
set -eu
: "${VERSION:?Run make artifacts to resolve versions.yaml and prepare the UI}"
revision=$(cat .source-revision)
for arch in amd64 arm64; do
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" "${GO:-go}" build -buildvcs=false -trimpath \
        -ldflags "-s -w -X main.revision=$revision" \
        -o "dist/scitrera-auth-proxy_${VERSION}_linux_${arch}" ./cmd/scitrera-auth-proxy
done
cp internal/adminui/dist/source.tar.gz "dist/scitrera-auth-go_${VERSION}_source.tar.gz"
cd dist
sha256sum "scitrera-auth-proxy_${VERSION}_linux_amd64" \
    "scitrera-auth-proxy_${VERSION}_linux_arm64" \
    "scitrera-auth-go_${VERSION}_source.tar.gz" > SHA256SUMS
