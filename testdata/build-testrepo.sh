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
    --provenance=false \
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
  --provenance=false \
  -f "Dockerfile.v1" .
docker buildx build \
  -t "testrepo:v2" -o "type=oci,dest=v2.tar" \
  --build-context testrepo:b1=docker-image://busybox:latest \
  --build-arg "arg=build-v2" --build-arg "arg_label=arg_for_label" \
  --platform "linux/amd64,linux/arm64,linux/arm/v7" \
  --provenance=false \
  -f "Dockerfile.v2" .
docker buildx build \
  -t "testrepo:v3" -o "type=oci,dest=v3.tar" \
  --build-context testrepo:b1=docker-image://busybox:latest \
  --build-arg "arg=build-v3" --build-arg "arg_label=arg_for_label" \
  --platform "linux/amd64,linux/arm64,linux/arm/v7,linux/arm/v6" \
  --provenance=false \
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

# create a docker artifact on v1
echo scripted build | regctl artifact put \
  --media-type application/vnd.oci.image.manifest.v1+json \
  --artifact-type application/vnd.oci.image.config.v1+json \
  --file-media-type application/vnd.example.build-type+json \
  ocidir://testrepo:a-docker
regctl index add \
  --ref "ocidir://testrepo:a-docker" \
  --desc-annotation "vnd.docker.reference.type=builder" \
  --desc-annotation "vnd.docker.reference.digest=$(regctl image digest --platform linux/amd64 ocidir://testrepo:v1)" \
  --desc-platform "unknown/unknown" \
  ocidir://testrepo:v1
regctl index add \
  --ref "ocidir://testrepo:a-docker" \
  --desc-annotation "vnd.docker.reference.type=builder" \
  --desc-annotation "vnd.docker.reference.digest=$(regctl image digest --platform linux/arm64 ocidir://testrepo:v1)" \
  --desc-platform "unknown/unknown" \
  ocidir://testrepo:v1

# create two artifacts on v2
echo eggs | regctl artifact put \
  --artifact-type application/example.sbom -m application/example.sbom.breakfast \
  --subject ocidir://testrepo:v2 ocidir://testrepo:a1
echo signed | regctl artifact put \
  --artifact-type application/example.signature -m application/example.signature.text \
  --subject ocidir://testrepo:v2 ocidir://testrepo:a2
echo no arms | regctl artifact put \
  --artifact-type application/example.arms -m application/example.arms \
  --subject ocidir://testrepo:v2 --platform linux/amd64
echo no arms | regctl artifact put \
  --artifact-type application/example.arms -m application/example.arms \
  --subject ocidir://testrepo:v2 --platform linux/amd64
echo 7 arms | regctl artifact put \
  --artifact-type application/example.arms -m application/example.arms \
  --subject ocidir://testrepo:v2 --platform linux/arm/v7
echo 64 arms | regctl artifact put \
  --artifact-type application/example.arms -m application/example.arms \
  --subject ocidir://testrepo:v2 --platform linux/arm64

# create a digest tag from v3 pointing to v1
v1_dig="$(regctl image digest ocidir://testrepo:v1)"
v1_dig=${v1_dig#*:}
v3_dig="$(regctl image digest ocidir://testrepo:v3)"
v3_shortdig="$(echo ${v3_dig#*:} | cut -b1-16)"
regctl image copy ocidir://testrepo:v3 "ocidir://testrepo:sha256-${v1_dig}.${v3_shortdig}.meta"

rm b1.tar b2.tar b3.tar
