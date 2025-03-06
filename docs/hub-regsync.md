# regsync

regsync is a registry synchronization utility used to update mirrors of OCI compatible container registries.

## Important Links

- Project website: [regclient.org](https://regclient.org)
- Installation options: [regclient.org/install/](https://regclient.org/install/)
- CLI Reference: [regclient.org/cli/regsync/](https://regclient.org/cli/regsync/)
- Source: [github.com/regclient/regclient](https://github.com/regclient/regclient)
- Releases: [github.com/regclient/regclient/releases](https://github.com/regclient/regclient/releases)
- Contribution guidelines: [github.com/regclient/regclient - contributing.md](https://github.com/regclient/regclient/blob/main/CONTRIBUTING.md)

## Available Tags

- `regclient/regsync:latest`: Most recent release based on scratch.
- `regclient/regsync:alpine`: Most recent release based on alpine.
- `regclient/regsync:edge`: Most recent commit to the main branch based on scratch.
- `regclient/regsync:edge-alpine`: Most recent commit to the main branch based on alpine.
- `regclient/regsync:$ver`: Specific release based on scratch (see below for semver details).
- `regclient/regsync:$ver-alpine`: Specific release based on alpine (see below for semver details).

Scratch based images do not include a shell or any credential helpers.
Alpine based images are based on the latest pinned alpine image at the time of release and include credential helpers for AWS and Google Cloud.

Semver version values for `$ver` are based on the [GitHub tags](https://github.com/regclient/regclient/tags).
These versions also tag major and minor versions, e.g. a release for `v0.7.1` will also tag `v0.7` and `v0`.

## Docker Quick Start

### Setup a Registry

```shell
docker network create registry
docker run -d --restart=unless-stopped --name registry --net registry \
  -e "REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY=/var/lib/registry" \
  -e "REGISTRY_STORAGE_DELETE_ENABLED=true" \
  -e "REGISTRY_VALIDATION_DISABLED=true" \
  -v "registry-data:/var/lib/registry" \
  -p "127.0.0.1:5000:5000" \
  registry:2
```

### Configure a Sync Yaml

Create a file called `regsync.yml`:

```yaml
version: 1
creds:
  - registry: registry:5000
    tls: disabled
    scheme: http
  - registry: docker.io
    user: "{{env \"HUB_USER\"}}"
    pass: "{{file \"/var/run/secrets/hub_token\"}}"
defaults:
  ratelimit:
    min: 100
    retry: 15m
  parallel: 2
  interval: 60m
  backup: "bkup-{{.Ref.Tag}}"
sync:
  - source: busybox:latest
    target: registry:5000/library/busybox:latest
    type: image
  - source: alpine
    target: registry:5000/library/alpine
    type: repository
    tags:
      allow:
      - "latest"
      - "3"
      - "3.\\d+"
  - source: regclient/regctl:latest
    target: registry:5000/regclient/regctl:latest
    type: image
```

You'll also need to create a `hub_token` file that includes either your hub
password or a personal access token.

### Test regsync

Run regsync in the "once" mode to populate your registry according to the above
yaml. Make sure to replace `your_username` with your Hub username. Note that
this command will pull a number of images from Hub, but will automatically rate
limit itself if you have less than 100 pulls remaining on your account.

```shell
docker container run -it --rm --net registry \
  -v "$(pwd)/regsync.yml:/home/appuser/regsync.yml:ro" \
  -v "$(pwd)/hub_token:/var/run/secrets/hub_token:ro" \
  -e "HUB_USER=your_username" \
  regclient/regsync:latest -c /home/appuser/regsync.yml once
```

### Run regsync

Once the one time sync looks good, deploy a regsync service in the background,
again replacing `your_username`:

```shell
docker container run -d --restart=unless-stopped --name regsync --net registry \
  -v "$(pwd)/regsync.yml:/home/appuser/regsync.yml:ro" \
  -v "$(pwd)/hub_token:/var/run/secrets/hub_token:ro" \
  -e "HUB_USER=your_username" \
  regclient/regsync:latest -c /home/appuser/regsync.yml server
```

You can verify it started by checking the logs with `docker container logs
regsync`. In server mode, no logs will show until the next scheduled run. In
the above example, that would be 60 minutes. And then, the only output you'll
see is when a new image gets pulled.

### Use your registry

Now you can run images from your registry or build new images with the above base:

```shell
docker container run -it --rm localhost:5000/library/busybox echo hello world
```
