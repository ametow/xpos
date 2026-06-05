#!/usr/bin/env bash
# Cross-compile the xpos agent CLI for every supported platform. The
# agent binary entrypoint lives at ./cmd/agent.
set -euo pipefail

mkdir -p bin

build() {
    local goos=$1
    local goarch=$2
    local ext=${3:-}
    local out="bin/xpos-${goos}-${goarch}${ext}"
    GOOS=$goos GOARCH=$goarch go build -ldflags '-s' -o "$out" ./cmd/agent
    echo "built $out"
}

build darwin  amd64
build darwin  arm64
build linux   386
build linux   amd64
build linux   arm
build linux   arm64
build windows 386   .exe
build windows amd64 .exe
