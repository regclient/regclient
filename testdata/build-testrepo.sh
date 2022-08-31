#!/bin/sh

set -e
cd "$(dirname $0)"

# recreate testrepo
rm -r testrepo

# build base images
for i in 1 2 3; do
docker buildx build \
  -t "testrepo:b${i}" -o "type=oci,dest=b${i}.tar" \
  --platform "linux/amd64,linux/arm64,linux/arm/v7,linux/arm/v6" \
  -f "Dockerfile.b${i}" .
  regctl image import "ocidir://testrepo:b${i}" "b${i}.tar"
  regctl image mod \
    --annotation "org.example.version=b${i}" \
    --time-max 2020-01-01T00:00:00Z \
    --replace "ocidir://testrepo:b${i}"
  regctl image export "ocidir://testrepo:b${i}" "b${i}.tar"
done

# build 3 images
# use oci-layout after next release of docker enables it
# the hacky workaround is to use busybox and do a rebase in the image mod
#  --build-context testrepo:b1=oci-layout://b1.tar \
docker buildx build \
  -t "testrepo:v1" -o "type=oci,dest=v1.tar" \
  --build-context testrepo:b1=docker-image://busybox:latest \
  --build-arg "arg=build-v1" --build-arg "arg_label=arg_for_label" \
  --platform "linux/amd64,linux/arm64" \
  -f "Dockerfile.v1" .
docker buildx build \
  -t "testrepo:v2" -o "type=oci,dest=v2.tar" \
  --build-context testrepo:b1=docker-image://busybox:latest \
  --build-arg "arg=build-v2" --build-arg "arg_label=arg_for_label" \
  --platform "linux/amd64,linux/arm64,linux/arm/v7" \
  -f "Dockerfile.v2" .
docker buildx build \
  -t "testrepo:v3" -o "type=oci,dest=v3.tar" \
  --build-context testrepo:b1=docker-image://busybox:latest \
  --build-arg "arg=build-v3" --build-arg "arg_label=arg_for_label" \
  --platform "linux/amd64,linux/arm64,linux/arm/v7,linux/arm/v6" \
  -f "Dockerfile.v3" .

# add 3 images
for i in 1 2 3; do
  regctl image import "ocidir://testrepo:v${i}" "v${i}.tar"
  regctl image mod \
    --rebase-ref "busybox,ocidir://testrepo:b1" \
    --annotation "org.example.version=v${i}" \
    --time-max 2021-01-01T00:00:00Z \
    --replace "ocidir://testrepo:v${i}"
  rm "v${i}.tar"
done

# set the base image annotations
for i in 2 3; do
  regctl image mod \
    --annotation "org.opencontainers.image.base.name=ocidir://testrepo:b${i}" \
    --annotation "org.opencontainers.image.base.digest=$(regctl image digest ocidir://testrepo:b1)" \
    --replace "ocidir://testrepo:v${i}"
done

# create two artifacts on v2
echo eggs | regctl artifact put \
  --artifact-type application/example.sbom -m application/example.sbom.breakfast \
  --refers ocidir://testrepo:v2 ocidir://testrepo:a1
echo signed | regctl artifact put \
  --artifact-type application/example.signature -m application/example.signature.text \
  --refers ocidir://testrepo:v2 ocidir://testrepo:a2

# create a digest tag from v3 pointing to v1
v1_dig="$(regctl image digest ocidir://testrepo:v1)"
v1_dig=${v1_dig#*:}
v3_dig="$(regctl image digest ocidir://testrepo:v3)"
v3_shortdig="$(echo ${v3_dig#*:} | cut -b1-16)"
regctl image copy ocidir://testrepo:v3 "ocidir://testrepo:sha256-${v1_dig}.${v3_shortdig}.meta"

rm b1.tar b2.tar b3.tar
