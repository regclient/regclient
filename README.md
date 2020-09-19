# regclient

Client interface for the registry API.
This includes `regctl` for a command line interface to manage registries.

**This project is in early development, some features are not complete.**

## regctl Features

- Ability to inspect repo tags, manifests, and image configs without
  downloading the full image.
- Ability to copy or retag an image without pulling it into docker. Layers are
  only pulled if you are copying between different registries and the target
  registry does not have the layers already.
- Ability to export an image from a registry without a docker engine.
- Uses docker registry logins and /etc/docker/certs.d by default to support
  private repositories and self signed registries.

## Building

```shell
git clone https://github.com/regclient/regclient.git
cd regclient
go build -o regctl ./cmd/regctl/
```

## Running as a Container

You can run regctl completely isolated in a container:

```shell
docker container run -it --rm --net host \
  -v regctl-conf:/home/appuser/.regctl/ \
  regclient/regctl:latest --help
```

Or on Linux and Mac environments, you can run it as your own user and save
configuration settings, use docker credentials, and use any docker certs:

```shell
docker container run -it --rm --net host \
  -u "$(id -u):$(id -g)" -e HOME -v $HOME:$HOME \
  -v /etc/docker/certs.d:/etc/docker/certs.d:ro \
  regclient/regctl:latest --help
```

This can be packaged as a shell script with:

```shell
cat >regctl <<EOF
#!/bin/sh

docker container run -it --rm --net host \\
  -u "\$(id -u):\$(id -g)" -e HOME -v \$HOME:\$HOME \\
  -v /etc/docker/certs.d:/etc/docker/certs.d:ro \\
  regclient/regctl:latest "\$@"
EOF
chmod 755 regctl
./regctl --help
```

## Demo

```shell
$ ./regctl repo ls ubuntu | grep 20.10
library/ubuntu:20.10

$ ./regctl image manifest ubuntu:20.10
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
  "manifests": [
    {
      "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
      "size": 1152,
      "digest": "sha256:bb03a3e24da9704fc94ff11adbbfd9c93bb84bfab6fd57c9bab3168431a1d1ff",
      "platform": {
        "architecture": "amd64",
        "os": "linux"
      }
    },
    {
      "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
      "size": 1152,
      "digest": "sha256:eb1e82d1e85283abb47d328cb838e8adff126ec4f44398a0e4b7f66dbad3fcb3",
      "platform": {
        "architecture": "arm",
        "os": "linux",
        "variant": "v7"
      }
    },
...

$ ./regctl image inspect ubuntu:20.10@sha256:bb03a3e24da9704fc94ff11adbbfd9c93bb84bfab6fd57c9bab3168431a1d1ff
{
  "created": "2020-08-19T21:15:25.559275011Z",
  "architecture": "amd64",
  "os": "linux",
  "config": {
    "Env": [
      "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    ],
    "Cmd": [
      "/bin/bash"
    ]
  },
  "rootfs": {
    "type": "layers",
    "diff_ids": [
      "sha256:327d7aa9de643a7e07b8a258fce4f0103a1a997112abd9ca13ce42c326aae474",
      "sha256:08b4849c15c3c5a7feaaf7bbe5cc7e82a83e6411b2aaf491884dc7f036b070af",
      "sha256:a1beb1d2d31d68cb8987e38c8170a615968d7ba46c0c6b311d36e2891f849b70",
      "sha256:b351340d34bcaf409df1cffdda8172a2a296635d16ba1ca74e1cd27cbfcf8d2b"
    ]
  },
  "history": [
    {
      "created": "2020-08-19T21:15:22.638202373Z",
      "created_by": "/bin/sh -c #(nop) ADD file:53ca8a3f446b0751019d522066ce844f6281ffb5b15e9605cd8940176abf4c76 in / "
    },
...
```

## Comparison to Other Tools

Registry client API:

- containerd: containerd'd registry APIs focus more on pulling images than on a general purpose registry client API. This means various registry API calls are not provided.
- docker/distribution: Docker's client libraries would have needed a fair bit of modification to support OCI images, and behave similar to the docker command line with registry logins.

There are also a variety of registry command line tools available:

- genuinetools/img: img works on top of buildkit for image creation and management. Using this for a registry client means including lots of dependencies that many will not need.
- genuinetools/reg: reg is probably the closest match to this project. Some features included in regctl that aren't included in reg are the ability to inject self signed certs, store login credentials separate from docker, copy or retag images, and export images into a tar file.
- containers/skopeo: Because of RedHat's push to remove any docker solutions from their stack, their skopeo project wasn't considered when searching for a complement to the docker command line.
