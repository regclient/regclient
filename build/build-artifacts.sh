#!/bin/sh

proj_dir="$(dirname "$0")/../"
cd "$proj_dir"
mkdir -p "artifacts"

targets="linux/amd64 linux/arm64 darwin/amd64 windows/amd64"

for target in $targets; do
  GOOS="${target%%/*}"
  GOARCH="${target#*/}"
  export GOOS GOARCH
  echo "Building regctl-${GOOS}-${GOARCH}"
  go build -o "artifacts/regctl-${GOOS}-${GOARCH}" ./cmd/regctl/
done
