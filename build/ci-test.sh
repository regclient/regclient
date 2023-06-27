#!/bin/sh

set -e

# CLI options for source/target ref
opt_s=ocidir://testdata/testrepo
opt_t=localhost:5000/ci-test
opt_h=0
while getopts 'hs:t:' option; do
  case $option in
    h) opt_h=1;;
    s) opt_s="$OPTARG";;
    t) opt_t="$OPTARG";;
  esac
done
set +e
shift $(expr $OPTIND - 1)
if [ $# -gt 0 -o "$opt_h" = "1" ]; then
  echo "Usage: $0 [opts]"
  echo " -h: this help message"
  echo " -s ref: source reference (default: $opt_s)"
  echo " -t ref: target reference"
  exit 1
fi
set -e

# cd to repo root, gather details from git, and build images
git_root="$(git rev-parse --show-toplevel)"
cd "${git_root}"
export PATH="$PATH:${git_root}/bin"

export REGCTL_CONFIG="${git_root}/.regctl_conf_ci.json"

# disable TLS for tests
if [ "${opt_s}" = "${opt_s#*://}" ]; then
  regctl registry set --tls=disabled "${opt_s%%/*}"
fi
if [ "${opt_t}" = "${opt_t#*://}" ]; then
  regctl registry set --tls=disabled "${opt_t%%/*}"
fi

regctl image copy --digest-tags --referrers "${opt_s}:v1" "${opt_t}:v1"
regctl image copy --digest-tags --referrers "${opt_s}:v2" "${opt_t}:v2"
regctl image copy --digest-tags --referrers "${opt_s}:v3" "${opt_t}:v3"

regctl image copy --digest-tags --referrers "${opt_t}:v2" "${opt_t}:v2.1-rc1"
regctl image copy --digest-tags --referrers "${opt_t}:v2" "${opt_t}:v2.1"

tags="$(regctl tag ls "${opt_t}")"
for tag in v1 v2 v3 v2.1-rc1 v2.1; do
  if ! echo "$tags" | grep -q "^$tag\$"; then
    echo "tag not found: $tag" >&2
    exit 1
  fi
done

regctl tag rm "${opt_t}:v2.1-rc1"
if regctl manifest head "${opt_t}:v2.1-rc1" >/dev/null 2>&1; then
  echo "tag not deleted: ${opt_t}:v2.1-rc1" >&2
  exit 1
fi

arm="$(regctl artifact get --platform linux/amd64 --subject "${opt_t}:v2" --filter-artifact-type application/example.arms)"
if [ "$arm" != "no arms" ]; then
  echo "failed to get artifact, expected no arms, received $arm"
fi

# TODO:
# - run other commands (manifest get, tag rm, image mod, index create, artifact put)
# - compare to testdata
# - get output from various commands, compare to expected

rm "${REGCTL_CONFIG}"
