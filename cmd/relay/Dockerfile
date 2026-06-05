# syntax=docker/dockerfile:1.7
# Multi-stage build for the xpos relay. Produces a small distroless
# image running as a non-root user. The agent has its own build path
# (build.sh) and is not produced here.

ARG GO_VERSION=1.23

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

# Cache deps separately from sources for faster rebuilds.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/xpos-relay \
        ./cmd/relay

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /
COPY --from=build /out/xpos-relay /xpos-relay

# Ports:
#   9876 - agent control / event server (now also carries multiplexed
#          per-connection yamux streams; protocol v2)
#   8080 - public HTTP gateway
#   9090 - admin (healthz/readyz/metrics)
# TCP tunnel public listeners are still bound dynamically on :0; the
# operator-managed port-range allocator (Phase 6 TCPRoute work) will
# replace this with a fixed range exposed by per-pod Services.
EXPOSE 9876 8080 9090

USER nonroot:nonroot
ENTRYPOINT ["/xpos-relay"]
