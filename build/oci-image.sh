#!/bin/sh

set -e
image="regctl"
platforms="linux/386,linux/amd64,linux/arm/v6,linux/arm/v7,linux/arm64,linux/ppc64le,linux/s390x,linux/riscv64"
base_name=""
release="scratch"
push_tags=""
ALPINE_NAME="alpine:3"
ALPINE_DIGEST="sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659" # 3.23.3

# CLI options to override image, platform, base digest, and comma separated list of tags to push
opt_c=0
opt_h=0
while getopts 'b:cd:hi:p:r:t:' option; do
  case $option in
    b) base_name="$OPTARG";;
    c) opt_c=1;;
    d) base_digest="$OPTARG";;
    h) opt_h=1;;
    i) image="$OPTARG";;
    p) platforms="$OPTARG";;
    r) release="$OPTARG";;
    t) push_tags="$OPTARG";;
  esac
done
set +e
shift $(expr $OPTIND - 1)
if [ $# -gt 0 -o "$opt_h" = "1" ]; then
  echo "Usage: $0 [opts]"
  echo " -b: base image name"
  echo " -c: use cache"
  echo " -d: base image digest"
  echo " -h: this help message"
  echo " -i: image to build (${image})"
  echo " -p: platforms to build (${platforms})"
  echo " -r: release target (${release})"
  echo " -t: tags to push (comma separated image list)"
  exit 1
fi
set -e

# cd to repo root, gather details from git, and build images
git_root="$(git rev-parse --show-toplevel)"
cd "${git_root}"
export PATH="$PATH:${git_root}/bin"
now_date="$(date +%Y-%m-%dT%H:%M:%SZ --utc)"
vcs_sec="$(git log -1 --format=%ct)"
vcs_date="$(date -d "@${vcs_sec}" +%Y-%m-%dT%H:%M:%SZ --utc)"
vcs_repo="https://github.com/regclient/regclient.git"
vcs_sha="$(git rev-list -1 HEAD)"
if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
  vcs_sha="${vcs_sha}-dirty"
fi
vcs_describe="$(git describe --all)"
vcs_version="(devel)"
if [ "${vcs_describe}" != "${vcs_describe#tags/}" ]; then
  vcs_version="${vcs_describe#tags/}"
elif [ "${vcs_describe}" != "${vcs_describe#heads/}" ]; then
  vcs_version="${vcs_describe#heads/}"
  if [ "main" = "${vcs_version}" ]; then
    vcs_version=edge
  fi
fi
vcs_version="$(echo "${vcs_version}" | sed -r 's#/+#-#g')"

build_opts=""
if [ -n "$base_name" ] && [ -z "$base_digest" ]; then
  if [ "$base_name" = "${ALPINE_NAME}" ]; then
    base_digest="${ALPINE_DIGEST}"
  else
    base_digest="$(regctl image digest "${base_name}")"
  fi
  echo "Base image digest: ${base_digest}"
elif [ -n "$base_name" ] && [ -n "$base_digest" ]; then
  build_opts=--build-context "${base_name}=docker-image://${base_name}@${base_digest}"
fi
[ -d "output" ] || mkdir -p output
if [ "${opt_c}" = "0" ]; then
  build_opts="$build_opts --no-cache"
fi
docker buildx build --platform="$platforms" \
  -f "build/Dockerfile.${image}.buildkit" \
  -o "type=oci,oci-artifact=true,dest=output/${image}-${release}.tar" \
  --provenance version=v1,mode=max \
  --target "release-${release}" \
  --build-arg "SOURCE_DATE_EPOCH=${vcs_sec}" \
  --build-arg "BUILD_DATE=${vcs_date}" \
  --build-arg "VCS_REF=${vcs_sha}" \
  --build-arg "VCS_VERSION=${vcs_version}" \
  ${build_opts} .

echo "Importing tar"
regctl tag rm "ocidir://output/${image}:${release}" 2>/dev/null || true
regctl image import "ocidir://output/${image}:${release}" "output/${image}-${release}.tar"

echo "Modding image"
regctl image mod \
  "ocidir://output/${image}:${release}" --replace \
  --to-oci-referrers --label-to-annotation --annotation-promote
if [ -n "$base_name" ] && [ -n "$base_digest" ]; then
  regctl image mod \
    "ocidir://output/${image}:${release}" --replace \
    --annotation "[*]org.opencontainers.image.base.name=${base_name}" \
    --annotation "[*]org.opencontainers.image.base.digest=${base_digest}" \
    --reproducible \
    --time "set=${vcs_date},base-ref=${base_name}@${base_digest}" \
    >/dev/null
else
  regctl image mod \
    "ocidir://output/${image}:${release}" --replace \
    --reproducible \
    --time "set=${vcs_date}" \
    >/dev/null
fi

# attach sboms to each platform
if command -v syft >/dev/null; then
  for digest in $(regctl manifest get ocidir://output/${image}:${release} --format '{{range .Manifests}}{{printf "%s\n" .Digest}}{{end}}'); do
    echo "Attaching SBOMs for ${image}:${release}@${digest}"
    regctl image copy ocidir://output/${image}@${digest} ocidir://output/${image}-sbom -v warn >/dev/null
    syft scan -q "oci-dir:output/${image}-sbom" --source-name "docker:docker.io/regclient/${image}@${digest}" -o cyclonedx-json \
      | regctl artifact put --subject "ocidir://output/${image}@${digest}" \
          --artifact-type application/vnd.cyclonedx+json \
          -m application/vnd.cyclonedx+json \
          --annotation "org.opencontainers.image.created=${now_date}" \
          --annotation "org.opencontainers.image.description=CycloneDX JSON SBOM"
    syft scan -q "oci-dir:output/${image}-sbom" --source-name "docker:docker.io/regclient/${image}@${digest}" -o spdx-json \
      | regctl artifact put --subject "ocidir://output/${image}@${digest}" \
          --artifact-type application/spdx+json \
          -m application/spdx+json \
          --annotation "org.opencontainers.image.created=${now_date}" \
          --annotation "org.opencontainers.image.description=SPDX JSON SBOM"
    rm -r output/${image}-sbom
  done
else
  echo "\033[31mWARNING:\033[0m Skipping SBOM generation because syft is not installed." >&2
fi

# manually prune old digest tags from previous builds and before the mod
for tag in $(regctl tag ls ocidir://output/${image}); do
  if [ "${tag}" != "${tag#sha256-}" ] && ! regctl manifest head "ocidir://output/${image}@sha256:${tag#sha256-}" 2>/dev/null; then
    regctl tag rm "ocidir://output/${image}:${tag}"
  fi
done

echo "\033[32mDigest for ${image}-${release}:\033[0m $(regctl image digest "ocidir://output/${image}:${release}")"

# split tags by comma and push each tag
if [ -n "$push_tags" ]; then
  for push_tag in $(echo "$push_tags" | tr , " "); do
    echo "Push: ${push_tag}"
    regctl image copy -v info "ocidir://output/${image}:${release}" "${push_tag}"
  done
fi
