# SPDX-License-Identifier: AGPL-3.0-only
FROM node:24.13.0-bookworm-slim@sha256:4660b1ca8b28d6d1906fd644abe34b2ed81d15434d26d845ef0aced307cf4b6f AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
WORKDIR /src
COPY . .
RUN cd web && npm run build

FROM golang:1.25.14-bookworm@sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437 AS build
ENV GOWORK=off GOTOOLCHAIN=local CGO_ENABLED=0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY --from=web /src /src
# The Go version declaration is synchronized from versions.yaml by repo-tools.
RUN go build -buildvcs=false -trimpath -ldflags "-s -w -X main.revision=$(cat .source-revision)" -o /out/scitrera-auth-proxy ./cmd/scitrera-auth-proxy

FROM alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40
# Generated version mirror; edit versions.yaml and run make versions.
ARG VERSION=0.1.0-rc.1
ARG REVISION=local-review
LABEL org.opencontainers.image.title="Scitrera Auth" \
      org.opencontainers.image.source="https://github.com/scitrera/scitrera-auth-go" \
      org.opencontainers.image.version=$VERSION \
      org.opencontainers.image.revision=$REVISION \
      org.opencontainers.image.licenses="AGPL-3.0-only AND Apache-2.0"
COPY --from=build /out/scitrera-auth-proxy /usr/local/bin/scitrera-auth-proxy
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=web /src/LICENSE /src/NOTICE /src/THIRD_PARTY_NOTICES.md /usr/share/doc/scitrera-auth/
COPY --from=web /src/LICENSES /usr/share/doc/scitrera-auth/LICENSES
USER 10001:10001
EXPOSE 8080 8081 8082
ENTRYPOINT ["/usr/local/bin/scitrera-auth-proxy"]
