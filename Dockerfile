# syntax=docker/dockerfile:1.7
#
# Multi-stage build for the Agora controller image.
#
# The controller is a single Go binary (cmd/agora) with the dashboard
# (ui/) embedded at compile time via ui/embed.go (//go:embed dist). The
# UI must therefore be built BEFORE the Go build runs.
#
# The same image is reused for one-shot Jobs (e.g. `admin store migrate`,
# `admin bootstrap`) by overriding CMD at deploy time.

# ---------------------------------------------------------------------------
# Stage 1: build the dashboard.
# ---------------------------------------------------------------------------
FROM node:22-bookworm-slim AS ui-build
WORKDIR /src/ui

COPY ui/package.json ui/package-lock.json ./
RUN npm ci

COPY ui/ ./
RUN npm run build \
 && test -d dist && test -n "$(ls -A dist)"

# ---------------------------------------------------------------------------
# Stage 2: build the agora controller binary.
# ---------------------------------------------------------------------------
FROM golang:1.25-bookworm AS go-build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
COPY --from=ui-build /src/ui/dist ./ui/dist

ARG TARGETOS=linux
ARG TARGETARCH=amd64

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
      -trimpath \
      -ldflags="-s -w" \
      -o /out/agora \
      ./cmd/agora

# ---------------------------------------------------------------------------
# Stage 3: minimal runtime image.
# ---------------------------------------------------------------------------
FROM alpine:3.20 AS runtime

RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S agora \
 && adduser -S -G agora -u 10001 agora \
 && mkdir -p /etc/agora /var/lib/agora \
 && chown -R agora:agora /etc/agora /var/lib/agora

COPY --from=go-build /out/agora /usr/local/bin/agora

USER agora:agora
WORKDIR /var/lib/agora

EXPOSE 8080

ENTRYPOINT ["agora"]
CMD ["controller", "/etc/agora/agora-controller.yaml"]
