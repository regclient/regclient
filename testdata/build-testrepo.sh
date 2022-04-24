#!/bin/sh

cd "$(dirname $0)"

# build 3 images
docker buildx build \
  -t "testrepo:v1" -o "type=oci,dest=v1.tar" \
  --build-arg "arg=build-v1" --build-arg "arg_label=arg_for_label" \
  --platform "linux/amd64,linux/arm64" \
  -f "Dockerfile.v1" .
docker buildx build \
  -t "testrepo:v2" -o "type=oci,dest=v2.tar" \
  --build-arg "arg=build-v2" --build-arg "arg_label=arg_for_label" \
  --platform "linux/amd64,linux/arm64,linux/arm/v7" \
  -f "Dockerfile.v2" .
docker buildx build \
  -t "testrepo:v3" -o "type=oci,dest=v3.tar" \
  --build-arg "arg=build-v3" --build-arg "arg_label=arg_for_label" \
  --platform "linux/amd64,linux/arm64,linux/arm/v7,linux/arm/v6" \
  -f "Dockerfile.v3" .

# recreate testrepo with reproducible timestamps
rm -r testrepo
for i in v1 v2 v3; do
  regctl image import "ocidir://testrepo:${i}" "${i}.tar"
  regctl image mod \
    --annotation "org.example.version=${i}" \
    --time-max 2021-01-01T00:00:00Z \
    --replace "ocidir://testrepo:${i}"
  rm "${i}.tar"
done

# create a digest tag from v3 pointing to v1
v1_dig="$(regctl image digest ocidir://testrepo:v1)"
v1_dig=${v1_dig#*:}
v3_dig="$(regctl image digest ocidir://testrepo:v3)"
v3_shortdig="$(echo ${v3_dig#*:} | cut -b1-16)"
regctl image copy ocidir://testrepo:v3 "ocidir://testrepo:sha256-${v1_dig}.${v3_shortdig}.meta"
