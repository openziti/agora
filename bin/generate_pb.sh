#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_BIN="$(mktemp -d)"
trap 'rm -rf "$TMP_BIN"' EXIT

echo "...generating protobuf code"
GOBIN="$TMP_BIN" go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
GOBIN="$TMP_BIN" go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

PATH="$TMP_BIN:$PATH" protoc \
  -I "$ROOT_DIR" \
  --go_out="$ROOT_DIR" \
  --go_opt=paths=source_relative \
  --go-grpc_out="$ROOT_DIR" \
  --go-grpc_opt=paths=source_relative \
  "$ROOT_DIR/internal/networkagent/pb/network.proto"
