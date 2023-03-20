# Installing regctl, regsync, and regbot

- [Building From Source](#building-from-source)
- [Downloading Binaries](#downloading-binaries)
- [Running as a Container](#running-as-a-container)
- [Verifying Signatures](#verifying-signatures)
- [Reproducible Builds](#reproducible-builds)

## Building From Source

```shell
git clone https://github.com/regclient/regclient.git
cd regclient
make
bin/regctl version
```

## Downloading Binaries

Binaries are available on the [releases page](https://github.com/regclient/regclient/releases).

The latest release can be downloaded using curl (adjust "regctl" and
"linux-amd64" for the desired command and your own platform):

```shell
curl -L https://github.com/regclient/regclient/releases/latest/download/regctl-linux-amd64 >regctl
chmod 755 regctl
```

Merges into the main branch also have binaries available as artifacts within [GitHub Actions](https://github.com/regclient/regclient/actions/workflows/go.yml?query=branch%3Amain)

## Running as a Container

You can run `regctl`, `regsync`, and `regbot` in a container.

For `regctl` (include a `-t` for any commands that require a tty, e.g. `registry login`):

```shell
docker container run -i --rm --net host \
  -v regctl-conf:/home/appuser/.regctl/ \
  ghcr.io/regclient/regctl:latest --help
```

For `regsync`:

```shell
docker container run -i --rm --net host \
  -v "$(pwd)/regsync.yml:/home/appuser/regsync.yml" \
  ghcr.io/regclient/regsync:latest -c /home/appuser/regsync.yml check
```

For `regbot`:

```shell
docker container run -i --rm --net host \
  -v "$(pwd)/regbot.yml:/home/appuser/regbot.yml" \
  ghcr.io/regclient/regbot:latest -c /home/appuser/regbot.yml once --dry-run
```

Or on Linux and Mac environments, you can run `regctl` as your own user and save
configuration settings, use docker credentials, and use any docker certs:

```shell
docker container run -i --rm --net host \
  -u "$(id -u):$(id -g)" -e HOME -v $HOME:$HOME \
  -v /etc/docker/certs.d:/etc/docker/certs.d:ro \
  ghcr.io/regclient/regctl:latest --help
```

And `regctl` can be packaged as a shell script with:

```shell
cat >regctl <<EOF
#!/bin/sh
opts=""
case "\$*" in
  "registry login"*) opts="-t";;
esac
docker container run \$opts -i --rm --net host \\
  -u "\$(id -u):\$(id -g)" -e HOME -v \$HOME:\$HOME \\
  -v /etc/docker/certs.d:/etc/docker/certs.d:ro \\
  ghcr.io/regclient/regctl:latest "\$@"
EOF
chmod 755 regctl
./regctl --help
```

Images are also included with an alpine base, which are useful for CI pipelines that expect the container to include a `/bin/sh`.
These alpine based images also include the `ecr-login` and `gcr` credential helpers.

## Verifying Signatures

Binaries and images have been signed with cosign.

For images:

```shell
cosign verify \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp https://github.com/regclient/regclient/.github/workflows/ \
  ghcr.io/regclient/regctl:latest
```

For binaries:

```shell
curl -L https://github.com/regclient/regclient/releases/latest/download/regctl-linux-amd64 >regctl
chmod 755 regctl
curl -L https://github.com/regclient/regclient/releases/latest/download/metadata.tgz >metadata.tgz
tar -xzf metadata.tgz regctl-linux-amd64.pem regctl-linux-amd64.sig
cosign verify-blob \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp https://github.com/regclient/regclient/.github/workflows/ \
  --certificate regctl-linux-amd64.pem \
  --signature regctl-linux-amd64.sig \
  regctl
rm metadata.tgz regctl-linux-amd64.pem regctl-linux-amd64.sig
```

## Reproducible Builds

Images can be rebuilt reproducibly.
This requires the following:

- source code to be cloned locally
- docker with buildx
- regctl
- syft

```shell
make oci-image

# compare regctl digests to edge/main
regctl image digest ocidir://output/regctl:scratch
regctl image digest ghcr.io/regclient/regctl:edge
regctl image digest ocidir://output/regctl:alpine
regctl image digest ghcr.io/regclient/regctl:edge-alpine

# compare regsync digests to edge/main
regctl image digest ocidir://output/regsync:scratch
regctl image digest ghcr.io/regclient/regsync:edge
regctl image digest ocidir://output/regsync:alpine
regctl image digest ghcr.io/regclient/regsync:edge-alpine

# compare regbot digests to edge/main
regctl image digest ocidir://output/regbot:scratch
regctl image digest ghcr.io/regclient/regbot:edge
regctl image digest ocidir://output/regbot:alpine
regctl image digest ghcr.io/regclient/regbot:edge-alpine
```
