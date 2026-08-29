#!/usr/bin/env bash

# Copyright the regclient contributors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e
branch=""
tag=""
prev_tag=""
opt_dry_run=0
opt_help=0
gh_repo="regclient/regclient"
gh_branch="main"
gh_auth=""

# CLI options to override image, platform, base digest, and comma separated list of tags to push
opt_c=0
opt_h=0
while getopts 'dhp:t:' option; do
  case $option in
    d) opt_dry_run=1;;
    h) opt_help=1;;
    p) prev_tag="$OPTARG";;
    t) tag="$OPTARG";;
  esac
done
set +e
shift $(expr $OPTIND - 1)
if [ $# -gt 0 ] || [ "$opt_help" = "1" ]; then
  echo "Usage: $0 [opts]"
  echo " -d: dry run"
  echo " -h: this help message"
  echo " -p tag: previous tag for generating change list"
  echo " -t tag: tag to set"
  exit 1
fi
set -e

# cd to base of the git repo this script is located within
cd "$(dirname $0)"
cd "$(git rev-parse --show-toplevel)"

generate_changelog() {
  echo -e "## Release ${1}\n\nChanges:\n"
  hashes="$(git log --reverse --merges --format="%h" "${prev_tag:+${prev_tag}..}HEAD")"
  prs=""
  users=""
  for hash in ${hashes}; do
    subj="$(git show --format=%s ${hash})"
    pr="${subj#Merge pull request #}"
    if [ "$pr" != "$subj" ]; then
      pr="${pr%% *}"
      inc_pr=0
      msg="$(get_pr_changelog "${pr}")"
      if [ -n "$msg" ]; then
        echo "$msg"
        prs="${prs} ${pr}"
      fi
      users="${users}\n$(get_pr_user "${pr}")"
    fi
  done
  echo -e "\nContributors:\n"
  for user in $(echo -e "$users" | sort -u); do
    if [ -n "$user" ]; then
      echo "- @${user}"
    fi
  done
  echo
  for pr in ${prs}; do
    echo "[pr-${pr}]: https://github.com/${gh_repo}/pull/${pr}"
  done
}

if [ -x "$(command -v gh-token)" ]; then
  gh_auth="Authorization: Bearer $(gh-token)"
fi
get_pr_changelog() {
  # the greps are ugly, better parsing options are welcome
  curl -sL ${gh_auth:+-H "${gh_auth}"} \
    -H "Accept: application/vnd.github+json" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    "https://api.github.com/repos/${gh_repo}/pulls/${1}" \
  | jq -r .body \
  | grep -A 99 '### Changelog text' \
  | grep -B 99 '### Please verify' \
  | grep -v -e '^###' -e '<!--' -e '^\s*$' \
  | sed -e 's/^- //' -e 's/^/- /' -e 's/\r//' -e "s/\$/ ([PR ${1}][pr-${1}])/"
}

get_pr_user() {
  curl -sL ${gh_auth:+-H "${gh_auth}"} \
    -H "Accept: application/vnd.github+json" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    "https://api.github.com/repos/${gh_repo}/pulls/${1}" \
  | jq -r .user.login
}

# prompt with last tag, asking for next tag, defaulting to patch update
if [ -z "$prev_tag" ]; then
  prev_tag="$(git tag -l | grep -v -- "-rc" | tail -1)"
fi
# for dry-run, output the change list from the prev_tag to main and stop
if [ "$opt_dry_run" = "1" ]; then
  generate_changelog "dry run"
  exit 0
fi
if [ -z "$tag" ] || git show-ref "refs/tags/${tag}" --quiet; then
  # extract patch version from prev_tag
  next_tag=v0.1.0
  if [ -n "${prev_tag}" ]; then
    next_patch="$(expr 1 + "${prev_tag##*.}")"
    next_tag="${prev_tag%.*}.${next_patch}"
  fi
  read -p "Enter release to create [${next_tag}]: " tag
  if [ -z "$tag" ]; then
    tag="$next_tag"
  fi
  if git show-ref "refs/tags/${tag}" --quiet; then
    echo "Release already exists: ${tag}" >&2
    exit 1
  fi
fi

# validate tag syntax (v1.2.3-rc1)
if [[ ! ${tag} =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-.*|$) ]]; then
  echo -e "Unexpected tag syntax: \"${tag}\"" >&2
  exit 1
fi

# query for all logs since last tag, extract PR list with PR number and title
# look into pulling PR text from GH and extracting change log message
# format the log output to extract the PR number and commit id, don't show local branches
echo "Generating changelog..."
echo -e "# Release Notes\n" >RELEASE.md-next
generate_changelog ${tag} | tee -a RELEASE.md-next

# update the release notes with the newest release at the top of the file
echo >>RELEASE.md-next
if [ -f RELEASE.md ]; then
  tail +3 RELEASE.md >>RELEASE.md-next
fi
mv RELEASE.md-next RELEASE.md

# prompt user on the next steps
remote="$(git remote -v show | awk "\$2~/github.com:${gh_repo%/*}\/${gh_repo##*/}/{print \$1; exit}")"
remote="${remote:-origin}"
cat <<EOF
# Verify and update the RELEASE.md, then push it in a PR.
# Once the PR has merged, tag the release with:
git checkout "${gh_branch}"
git pull "${remote}" "${gh_branch}"
git tag -asm "Release ${tag}" "${tag}"
git push "${remote}" "${tag}"
EOF
