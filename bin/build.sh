#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="${OUTPUT_DIR:-$ROOT_DIR/build}"
if [[ "$OUTPUT_DIR" != /* ]]; then
	OUTPUT_DIR="$ROOT_DIR/$OUTPUT_DIR"
fi
OUTPUT_FILE="$OUTPUT_DIR/new-api-linux-amd64"

for command_name in go bun git; do
	if ! command -v "$command_name" >/dev/null 2>&1; then
		printf 'Required command not found: %s\n' "$command_name" >&2
		exit 1
	fi
done

version="${VERSION:-}"
if [[ -z "$version" && -f "$ROOT_DIR/VERSION" ]]; then
	version="$(<"$ROOT_DIR/VERSION")"
fi
if [[ -z "$version" ]]; then
	version="$(git -C "$ROOT_DIR" describe --tags --match 'v[0-9]*' --always 2>/dev/null || true)"
fi
if [[ -z "$version" ]]; then
	version="dev"
fi

printf 'Building frontend (version %s)...\n' "$version"
(
	cd "$ROOT_DIR/web"
	bun install --frozen-lockfile
	DISABLE_ESLINT_PLUGIN=true VITE_REACT_APP_VERSION="$version" bun run build
)

printf 'Downloading Go modules...\n'
(
	cd "$ROOT_DIR"
	GOWORK=off go mod download
)

mkdir -p "$OUTPUT_DIR"
printf 'Building Ubuntu 24.04-compatible Linux amd64 binary...\n'
(
	cd "$ROOT_DIR"
	GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags "-s -w -X github.com/QuantumNous/new-api/common.Version=$version" \
		-o "$OUTPUT_FILE" .
)

chmod 755 "$OUTPUT_FILE"
printf 'Build complete: %s\n' "$OUTPUT_FILE"
