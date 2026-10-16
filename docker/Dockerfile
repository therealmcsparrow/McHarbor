# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.

# Stage 1: the Go backend. Plugins are built and published separately.
ARG GO_BASE=golang:1.27.2-bookworm
FROM ${GO_BASE} AS backend
RUN apt-get update \
    && apt-get install -y --no-install-recommends git gcc libc6-dev \
    && apt-get clean && find /var/lib/apt/lists -mindepth 1 -delete
COPY src/backend /sdk/src/backend
RUN --mount=type=cache,target=/go/pkg/mod \
    cd /sdk/src/backend && go mod download
COPY src/plugins/ThemePacks/theme-pack-classic /sdk/src/plugins/ThemePacks/theme-pack-classic
COPY src/frontend/public/themes /sdk/src/frontend/public/themes
COPY src/plugins/theme-catalog /sdk/src/plugins/theme-catalog
# Plugins are no longer built here. They ship from the catalog at
# https://www.mcharbor.eu/catalog/plugins and are compiled by
# docker/Dockerfile.plugin-packager, which stages plugin source inside the
# host module tree. Because this stage now builds the host alone, with no
# go.work workspace, both sides resolve core/plugin through the same plain
# module graph and their per-package ABI hashes match - which is what
# plugin.Open checks. Reintroducing go.work here would break that.
#
# Only the theme pack is still packaged in-image: it ships no .so, so it is
# ABI-independent and costs a few KB.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd /sdk/src/backend && \
      CGO_ENABLED=1 go build -trimpath -o /mcharbor ./ && \
      mkdir -p /themes-catalog && \
      cp /sdk/src/plugins/theme-catalog/*.json /themes-catalog/ && \
      for plugin in theme-pack-classic; do \
        version=$(awk '/^version:[[:space:]]*/{print $2; exit}' /sdk/src/plugins/ThemePacks/$plugin/plugin.yaml | tr -d '"') && \
        rm -rf /tmp/theme-plugin-payload && \
        mkdir -p /tmp/theme-plugin-payload/themes && \
        cp /sdk/src/plugins/ThemePacks/$plugin/plugin.yaml /tmp/theme-plugin-payload/ && \
        cp /sdk/src/frontend/public/themes/*.css /tmp/theme-plugin-payload/themes/ && \
        tar -C /tmp/theme-plugin-payload -czf /themes-catalog/$plugin-$version.tar.gz .; \
      done && \
      ls -la /mcharbor

# Stage 2: React frontend
# Stage 1b: the remote agent, so McHarbor can serve the binary it was built
# with instead of whatever happens to be the latest GitHub release.
#
# The agent needs CGO against musl — libcontainer/nsenter carries a C
# constructor that runs before the Go runtime starts — which is why this is a
# separate stage from the pure-Go backend above rather than another line in it.
FROM golang:1.27.2-alpine AS agentbin
RUN apk add --no-cache gcc musl-dev libseccomp-dev libseccomp-static pkgconfig
WORKDIR /src/agent
COPY src/agent/ .
COPY src/cli /src/cli
COPY BUILD /BUILD
RUN MCHARBOR_BUILD="$(tr -d '[:space:]' < /BUILD)" && \
    go mod tidy && \
    CGO_ENABLED=1 \
    go build \
      -tags "seccomp urfave_cli_no_docs" \
      -ldflags "-s -w -extldflags=-static -X main.agentBuild=$MCHARBOR_BUILD" \
      -o /mcharbor-agent-linux-amd64 .

# An export-only stage holding nothing but the binary, so
# `--output type=local` writes one file. Exporting `agentbin` itself would
# try to recreate the whole Alpine filesystem, and its busybox symlinks need
# a privilege Windows does not grant an unelevated client.
FROM scratch AS agentexport
COPY --from=agentbin /mcharbor-agent-linux-amd64 /mcharbor-agent-linux-amd64

FROM --platform=$BUILDPLATFORM alpine:3.22 AS busybox-src
ARG BUSYBOX_VERSION=1.38.0
ARG BUSYBOX_SHA256=34f9ea6ff8636f2c9241153b9114eefa9e65674a45318ae1ef95bb5f31c53bb2
ARG BUSYBOX_PGP_FINGERPRINT=C9E9416F76E610DBD09D040F47B70C55ACC9965B
RUN apk add --no-cache curl gnupg bzip2
COPY signing/busybox-vda.linux.asc /tmp/busybox-signing.asc
RUN cd /tmp && \
    curl -fsSLo busybox.tar.bz2 "https://busybox.net/downloads/busybox-${BUSYBOX_VERSION}.tar.bz2" && \
    curl -fsSLo busybox.tar.bz2.sig "https://busybox.net/downloads/busybox-${BUSYBOX_VERSION}.tar.bz2.sig" && \
    echo "${BUSYBOX_SHA256}  busybox.tar.bz2" | sha256sum -c - && \
    export GNUPGHOME="$(mktemp -d)" && \
    gpg --batch --quiet --import /tmp/busybox-signing.asc && \
    gpg --batch --status-fd 1 --verify busybox.tar.bz2.sig busybox.tar.bz2 > verify.status && \
    grep -q "^\[GNUPG:\] VALIDSIG ${BUSYBOX_PGP_FINGERPRINT} " verify.status && \
    mkdir -p /src && tar -xjf busybox.tar.bz2 -C /src && \
    mv "/src/busybox-${BUSYBOX_VERSION}" /src/busybox && \
    rm -rf "$GNUPGHOME" busybox.tar.bz2 busybox.tar.bz2.sig verify.status

FROM --platform=linux/amd64 alpine:3.22 AS busybox-amd64
RUN apk add --no-cache build-base linux-headers perl
COPY --from=busybox-src /src/busybox /src/busybox
COPY docker/build-busybox.sh /usr/local/bin/build-busybox
ENV SOURCE_DATE_EPOCH=1778638914 KCONFIG_NOTIMESTAMP=1
RUN sh /usr/local/bin/build-busybox amd64

FROM --platform=linux/arm64 alpine:3.22 AS busybox-arm64
RUN apk add --no-cache build-base linux-headers perl
COPY --from=busybox-src /src/busybox /src/busybox
COPY docker/build-busybox.sh /usr/local/bin/build-busybox
ENV SOURCE_DATE_EPOCH=1778638914 KCONFIG_NOTIMESTAMP=1
RUN sh /usr/local/bin/build-busybox arm64

FROM scratch AS busyboxexport
COPY --from=busybox-amd64 /out/ /
COPY --from=busybox-arm64 /out/ /

FROM golang:1.27.2-bookworm AS cli
COPY src/cli /src/cli
COPY VERSION BUILD /src/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd /src/cli && \
      PKG=github.com/therealmcsparrow/mcharbor-cli/internal/cli && \
      CGO_ENABLED=0 GOWORK=off go build -trimpath \
        -ldflags "-s -w -X $PKG.Version=$(tr -d '[:space:]' < /src/VERSION) -X $PKG.Build=$(tr -d '[:space:]' < /src/BUILD)" \
        -o /mcharbor-cli .
COPY src/cli /repo/src/cli
COPY VERSION BUILD /repo/
COPY scripts/development/build-cli.sh /repo/scripts/development/build-cli.sh
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    apt-get update && apt-get install -y --no-install-recommends zip && rm -rf /var/lib/apt/lists/* && \
    bash /repo/scripts/development/build-cli.sh --out /cli-archives

FROM node:26-bookworm-slim AS frontend
WORKDIR /app
COPY src/frontend/package.json src/frontend/package-lock.json* ./
# --legacy-peer-deps lets the ESLint plugins Oxlint loads as jsPlugins
# (i18next, better-tailwindcss) install without the eslint peer they
# declare; Oxlint runs them without ESLint.
RUN npm install --legacy-peer-deps
COPY src/frontend/ .
COPY nodes/ /nodes/
COPY widgets/ /widgets/
RUN mkdir -p /src && ln -s /app /src/frontend
# app.css declares `@source "../plugins/Plugins/*/frontend/src/**"`, which
# from /app/app.css resolves to /plugins/Plugins/... . Without this copy the
# glob matches nothing and Tailwind silently drops every utility used only
# by a plugin frontend (e.g. the reporting scorecard's fill-* bars). Plugin
# frontend node_modules is excluded by .dockerignore.
COPY src/plugins/Plugins/ /plugins/Plugins/
COPY scripts/sanity/missing-api/plugin-routes.mjs scripts/sanity/missing-api/plugin-routes.d.mts /scripts/sanity/missing-api/
RUN node --disable-optimizing-compilers ./node_modules/typescript/bin/tsc -b \
    && node --disable-optimizing-compilers ./node_modules/vite-plus/bin/vp build

FROM scratch AS payload
COPY --from=backend /mcharbor /mcharbor
COPY --from=backend /themes-catalog /themes-catalog
COPY --from=frontend /app/dist /static
COPY --from=agentbin /mcharbor-agent-linux-amd64 /agent-bin/mcharbor-agent-linux-amd64
COPY --from=busybox-amd64 /out/busybox-amd64 /debug/busybox-amd64
COPY --from=busybox-arm64 /out/busybox-arm64 /debug/busybox-arm64
COPY signing/plugin-signing.pub /trust/plugin-signing.pub
COPY VERSION /VERSION
COPY BUILD /BUILD

# Stage 3: Production
FROM debian:bookworm-slim AS production
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        tini sqlite3 curl bash ca-certificates tzdata gnupg \
    && install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc \
    && chmod a+r /etc/apt/keyrings/docker.asc \
    && echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian bookworm stable" > /etc/apt/sources.list.d/docker.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends docker-ce-cli docker-compose-plugin \
    && apt-get clean && find /var/lib/apt/lists -mindepth 1 -delete

# Install Trivy vulnerability scanner
RUN curl -sfL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh | sh -s -- -b /usr/local/bin

# Install Grype vulnerability scanner
RUN curl -sSfL https://raw.githubusercontent.com/anchore/grype/main/install.sh | sh -s -- -b /usr/local/bin

WORKDIR /app

COPY --from=backend /mcharbor ./mcharbor

# The public half of the plugin signing key, and the env var that makes
# the host read it. It lives here rather than under $DATA_DIR because
# $DATA_DIR is a volume: a key written there would be shadowed the first
# time the image ran against an existing one.
COPY signing/plugin-signing.pub /app/trust/plugin-signing.pub
ENV MCHARBOR_PLUGIN_TRUST_KEYS=/app/trust/plugin-signing.pub
COPY --from=backend /themes-catalog /mnt/themes-catalog
COPY --from=frontend /app/dist ./static
COPY VERSION ./VERSION
COPY BUILD demo_mod[e] ./
# Served by GET /api/agent/binary/{os}/{arch}, so a deploy or a self-update
# installs this build rather than a GitHub release that may not exist.
COPY --from=agentbin /mcharbor-agent-linux-amd64 ./agent-bin/mcharbor-agent-linux-amd64
COPY --from=busybox-amd64 /out/busybox-amd64 ./debug/busybox-amd64
COPY --from=busybox-arm64 /out/busybox-arm64 ./debug/busybox-arm64
COPY --from=cli /mcharbor-cli /usr/local/bin/mcharbor
COPY --from=cli /cli-archives ./cli-bin
COPY docker/docker-entrypoint.sh /docker-entrypoint.sh

RUN chmod +x /docker-entrypoint.sh && mkdir -p /app/data

ENV PORT=8705 GODEBUG=cpu.avx512f=off,cpu.avx512vl=off
LABEL com.mcharbor.role=app
EXPOSE 8705

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f -k https://localhost:8705/api/health || curl -f http://localhost:8705/api/health || exit 1

ENTRYPOINT ["tini", "--", "/docker-entrypoint.sh"]
CMD ["./mcharbor"]
