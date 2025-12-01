#!/bin/sh

# TODO: support RCs
set -e
branch=""
tag=""
prev_tag=""
opt_dry_run=0
opt_help=0
gh_repo="regclient/regclient"
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
  echo "# Release ${1}\n\nChanges:\n"
  hashes="$(git log --reverse --merges --format="%h" "${prev_tag}..HEAD")"
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
      # msg="$(git show --format=%b ${hash})"
      # echo "- $msg ([PR ${pr}][pr-${pr}])"
    fi
  done
  echo "\nContributors:\n"
  for user in $(echo "$users" | sort -u); do
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
  prev_tag="$(git tag --sort=version:refname -l | tail -1)"
fi
# for dry-run, output the change list from the prev_tag to main and stop
# TODO: add a dry-run func to echo vs exec commands that would change things
if [ "$opt_dry_run" = "1" ]; then
  generate_changelog "dry run"
  exit 0
fi
if [ -z "$tag" ] || git show-ref "refs/tags/${tag}" --quiet; then
  # extract patch version from prev_tag
  next_patch="$(expr 1 + "${prev_tag##*.}")"
  next_tag="${prev_tag%.*}.${next_patch}"
  read -p "Enter tag to create [${next_tag}]: " tag
  if [ -z "$tag" ]; then
    tag="$next_tag"
  fi
  if git show-ref "refs/tags/${tag}" --quiet; then
    echo "Tag already exists: ${tag}" >&2
    exit 1
  fi
fi

# TODO: validate tag format (v1.2.3)

# check if branch exists, create if missing from the last release branch, else checkout
branch="${tag#v}"
branch="${branch%.*}"
if git show-ref "refs/heads/releases/${branch}"; then
  git checkout "releases/${branch}"
else
  prev_branch="${prev_tag#v}"
  prev_branch="${prev_branch%.*}"
  git checkout -b "releases/${branch}" "releases/${prev_branch}"
fi

# merge changes from main
git merge main -m "Merge for release ${tag}"

# query for all logs since last tag, extract PR list with PR number and title
# look into pulling PR text from GH and extracting change log message
# format the log output to extract the PR number and commit id, don't show local branches
echo "Generating changelog..."
generate_changelog ${tag} | tee release.md

# prompt user to make any changes to release.md and approve release
cat <<EOF
# Verify and update the release.md, then execute the following:
git add release.md
git commit -sm "Release ${tag}"
git push upstream releases/${branch}
git tag -asm "Release ${tag}" ${tag}
git push upstream ${tag}
git checkout main
EOF
