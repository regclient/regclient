#!/bin/sh

set -x
VCS_REF=$(git rev-list -1 HEAD)
LD_FLAGS="-X github.com/regclient/regclient/regclient.VCSRef=${VCS_REF}"
GO_BUILD_FLAGS=
proj_dir="$(dirname "$0")/../"
cd "$proj_dir"
mkdir -p "artifacts"

targets="linux/amd64 linux/arm64 darwin/amd64 windows/amd64"

for target in $targets; do
  GOOS="${target%%/*}"
  GOARCH="${target#*/}"
  export GOOS GOARCH
  echo "Building regctl-${GOOS}-${GOARCH}"
  go build -o "artifacts/regctl-${GOOS}-${GOARCH}" -ldflags "$LD_FLAGS" ${GO_BUILD_FLAGS} ./cmd/regctl/
done

targets="linux/amd64 linux/arm64"

for target in $targets; do
  GOOS="${target%%/*}"
  GOARCH="${target#*/}"
  export GOOS GOARCH
  echo "Building regsync-${GOOS}-${GOARCH}"
  go build -o "artifacts/regsync-${GOOS}-${GOARCH}" -ldflags "$LD_FLAGS" ${GO_BUILD_FLAGS} ./cmd/regsync/
done

for target in $targets; do
  GOOS="${target%%/*}"
  GOARCH="${target#*/}"
  export GOOS GOARCH
  echo "Building regbot-${GOOS}-${GOARCH}"
  go build -o "artifacts/regbot-${GOOS}-${GOARCH}" -ldflags "$LD_FLAGS" ${GO_BUILD_FLAGS} ./cmd/regbot/
done
