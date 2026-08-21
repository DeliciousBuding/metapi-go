# syntax=docker/dockerfile:1
#
# Multi-stage build for metapi-go.
# BuildKit cache mounts speed up CI by persisting the Go module/build caches
# and the Bun install cache across builds. Requires DOCKER_BUILDKIT=1 (default
# on Docker Engine 23+ and Docker Desktop).

# ARG declared before the first FROM is visible to every stage.
ARG VERSION=dev
# Build provenance surfaced by GET /api/about. No default: a plain
# `docker build` produces an empty commit/build time, which the About page
# renders as an em-dash instead of a fabricated SHA/timestamp. CI passes the
# real values (see .github/workflows/main.yml docker-push).
ARG COMMIT
ARG BUILD_TIME
# Single source of truth: .github/workflows/main.yml env.BUN_VERSION
# (docker-push/docker-build pass it as a build-arg; default matches).
ARG BUN_VERSION=1.3.14

# Stage 1: Frontend build (Bun + Rsbuild)
FROM oven/bun:${BUN_VERSION}-alpine AS web
WORKDIR /app/web
COPY web/package.json web/bun.lock ./
# --mount=type=cache keeps the Bun install cache across builds so repeat
# installs skip already-fetched tarballs.
RUN --mount=type=cache,target=/root/.bun/install-cache \
    bun install --frozen-lockfile
COPY web ./
RUN bun run build:web

# Stage 2: Go build
FROM golang:1.26.6-alpine AS build
ARG VERSION
ARG COMMIT
ARG BUILD_TIME
WORKDIR /app
COPY go.mod go.sum ./
# Module cache mount: `go mod download` becomes a no-op when go.sum hasn't
# changed, because every module is already in /go/pkg/mod.
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
# Narrow COPY: only the Go source the binary needs. web/src, web/node_modules,
# web/package.json etc. are never compiled by the Go toolchain and would only
# bust the build cache when the frontend changes. web/dist arrives from the
# frontend stage below.
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY store/ ./store/
COPY handler/ ./handler/
COPY router/ ./router/
COPY auth/ ./auth/
COPY config/ ./config/
COPY service/ ./service/
COPY platform/ ./platform/
COPY proxy/ ./proxy/
COPY routing/ ./routing/
COPY scheduler/ ./scheduler/
COPY app/ ./app/
COPY transform/ ./transform/
COPY e2e/ ./e2e/
COPY web/embed.go ./web/embed.go
COPY --from=web /app/web/dist ./web/dist
# Build cache mount: incremental Go compilation state survives across builds,
# so unchanged packages aren't recompiled even when the source context shifts.
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -trimpath -buildvcs=false \
    -ldflags="-s -w -X github.com/deliciousbuding/metapi-go/internal/version.Version=${VERSION} -X github.com/deliciousbuding/metapi-go/internal/version.Commit=${COMMIT} -X github.com/deliciousbuding/metapi-go/internal/version.BuildTime=${BUILD_TIME}" \
    -o metapi ./cmd/server
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -trimpath -buildvcs=false \
    -ldflags="-s -w -X github.com/deliciousbuding/metapi-go/internal/version.Version=${VERSION} -X github.com/deliciousbuding/metapi-go/internal/version.Commit=${COMMIT} -X github.com/deliciousbuding/metapi-go/internal/version.BuildTime=${BUILD_TIME}" \
    -o metapi-migrate ./cmd/migrate

# Stage 3: Runtime
FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -g 1001 -S appgroup && adduser -u 1001 -S appuser -G appgroup
RUN mkdir -p /app/data && chown -R appuser:appgroup /app/data
COPY --from=build /app/metapi /usr/local/bin/metapi
COPY --from=build /app/metapi-migrate /usr/local/bin/metapi-migrate
USER appuser
EXPOSE 4000
ENV HOST=0.0.0.0 \
    DATA_DIR=/app/data
VOLUME ["/app/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/usr/local/bin/metapi", "healthcheck"]
CMD ["/usr/local/bin/metapi"]
