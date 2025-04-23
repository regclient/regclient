# regctl

regctl is a CLI for accessing OCI compatible container registries.

## Important Links

- Project website: [regclient.org](https://regclient.org)
- Installation options: [regclient.org/install/](https://regclient.org/install/)
- CLI Reference: [regclient.org/cli/regctl/](https://regclient.org/cli/regctl/)
- Source: [github.com/regclient/regclient](https://github.com/regclient/regclient)
- Releases: [github.com/regclient/regclient/releases](https://github.com/regclient/regclient/releases)
- Contribution guidelines: [github.com/regclient/regclient - contributing.md](https://github.com/regclient/regclient/blob/main/CONTRIBUTING.md)

## Available Tags

- `regclient/regctl:latest`: Most recent release based on scratch.
- `regclient/regctl:alpine`: Most recent release based on alpine.
- `regclient/regctl:edge`: Most recent commit to the main branch based on scratch.
- `regclient/regctl:edge-alpine`: Most recent commit to the main branch based on alpine.
- `regclient/regctl:$ver`: Specific release based on scratch (see below for semver details).
- `regclient/regctl:$ver-alpine`: Specific release based on alpine (see below for semver details).

Scratch based images do not include a shell or any credential helpers.
Alpine based images are based on the latest pinned alpine image at the time of release and include credential helpers for AWS and Google Cloud.

Semver version values for `$ver` are based on the [GitHub tags](https://github.com/regclient/regclient/tags).
These versions also tag major and minor versions, e.g. a release for `v0.7.1` will also tag `v0.7` and `v0`.

## Docker Quick Start

```shell
docker container run -it --rm --net host \
  -v regctl-conf:/home/appuser/.regctl/ \
  regclient/regctl:latest --help
```

On Linux and Mac environments, here’s example shell script to run regctl in a container with settings for accessing content as your own user and using docker credentials and certificates:

```shell
#!/bin/sh
opts=""
case "$*" in
  "registry login"*) opts="-t";;
esac
docker container run $opts -i --rm --net host \
  -u "$(id -u):$(id -g)" -e HOME -v $HOME:$HOME \
  -v /etc/docker/certs.d:/etc/docker/certs.d:ro \
  ghcr.io/regclient/regctl:latest "$@"
```
