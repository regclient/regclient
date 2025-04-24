#!/bin/sh

set -e
cd "$(dirname $0)/.."

if [ $# != 1 ] || [ "$1" !=  "${1##-}" ]; then
  echo "usage: $0 [regclient_image_name]" >&2
  exit 1
fi

image_info="$(regctl manifest get "$1" --format '
  {{- println ( index .Annotations "org.opencontainers.image.revision" ) }} 
  {{- println ( index .Annotations "org.opencontainers.image.title" ) }} 
  {{- println ( index .Annotations "org.opencontainers.image.description" ) }} 
  {{- println .GetDescriptor.Digest }}')"

commit="$(echo "${image_info}" | sed -n 1p)"
sub_image="$(echo "${image_info}" | sed -n 2p)"
description="$(echo "${image_info}" | sed -n 3p)"
digest="$(echo "${image_info}" | sed -n 4p)"

# checkout the repo at the commit to reproduce
git switch -d "$commit" >/dev/null

# make the oci images
make "oci-image-${sub_image}"

scratch_desc="$(regctl manifest get ocidir://output/${sub_image}:scratch --format '{{ index .Annotations "org.opencontainers.image.description" }}')"
scratch_digest="$(regctl image digest ocidir://output/${sub_image}:scratch)"
alpine_desc="$(regctl manifest get ocidir://output/${sub_image}:alpine --format '{{ index .Annotations "org.opencontainers.image.description" }}')"
alpine_digest="$(regctl image digest ocidir://output/${sub_image}:alpine)"

rc=0
if [ "$description" = "$scratch_desc" ] && [ "$digest" = "$scratch_digest" ]; then
  echo "\033[32mScratch image reproduced.\033[0m"
  echo "$digest: $description"
elif [ "$description" = "$alpine_desc" ] && [ "$digest" = "$alpine_digest" ]; then
  echo "\033[32mAlpine image reproduced.\033[0m"
  echo "$digest: $description"
else
  echo "\033[31mFailed to reproduce!\033[0m"
  echo "Expected: $digest: $description"
  echo "Found:    $scratch_digest: $scratch_desc"
  echo "Found:    $alpine_digest: $alpine_desc"
  rc=2
fi

# revert the git repo
git switch - >/dev/null

exit $rc
