# RegClient Documentation

## regclient

API docs are still needed

## regctl

### Top level commands

```text
$ regctl --help
Utility for accessing docker registries
More details at https://github.com/regclient/regclient

Usage:
  regctl <cmd> [flags]
  regctl [command]

Available Commands:
  help        Help about any command
  image       manage images
  layer       manage image layers/blobs
  registry    manage registries
  repo        manage a repository

Flags:
  -h, --help               help for regctl
  -v, --verbosity string   Log level (debug, info, warn, error, fatal, panic) (default "warning")

Use "regctl [command] --help" for more information about a command.
```

### Registry commands

Registry commands allow configuring host regctl access a registry:

```text
Usage:
  regctl registry [command]

Available Commands:
  config      show registry config
  login       login to a registry
  logout      logout of a registry
  set         set options on a registry
```

With docker installed and logged into the registry, these commands are typically
not needed with the exception of configuring an insecure registry. The `regctl`
will import credentials from the docker logins stored in
`$HOME/.docker/config.json` and trust certificates loaded in
`/etc/docker/certs.d/$registry/*.crt`. These commands are useful for running in
an environment without docker to configure the `$HOME/.regctl/config.json` file.
One use case for that is to run `regctl` within an unpriviliged container in a
CI pipeline.

Note that it is possible to configure multiple registry servers under a single
name as a mirror with automatic failover. This is useful for pulling content,
but may have unexpected results when pushing changes to the registry since each
http request will be sent to the first server in the list that is currently
available.

### Tag commands

```text
Usage:
  regctl tag [command]

Available Commands:
  delete      delete a tag in a repo
  ls          list tags in a repo
```

The `ls` command lists all tags within a repo.

The `delete` command will delete a single tag without impacting other tags or
the underlying manifest which is useful if you are unsure if your image is used
elsewhere and want to rely on the registry to cleanup untagged manifests.

### Image Commands

The image commands are where most of the power of `regctl` is visible:

```text
Usage:
  regctl image [command]

Available Commands:
  copy        copy or retag image
  delete      delete image
  digest      show digest for pinning
  export      export image
  import      import image
  inspect     inspect image
  manifest    show manifest or manifest list
```

The `copy` command allows images to be copied between registries, between
repositories on the same registry, or retag an image within the same repository,
and only pulls the layers when needed (typically not needed with the same
registry server).

The `delete` command removes the image manifest from the server. This will
impact all tags pointing to the same manifest and requires a digest to be
included in the image reference to be deleted (e.g. `myimage@sha256:abcd...`).

The `digest` command is useful to pin the image used within your deployment to an
immutable sha256 checksum.

The `export`/import commands allow you to copy images between registry servers
that may be disconnected, or to export an image directly from a registry without
a docker engine and loading it into a potentially disconnected docker host.

The `inspect` command pulls the image config json blob. This is the same json
shown with a `docker image inspect` command, and includes labels, the
entrypoint/cmd, and layer history. This can be useful with image pruning
scripts, or other tools that need the image labels without the need to pull all
of the layers.

The `manifest` command shows the low level layers and digests that can be pulled
from the registry to retrieve individual components of an image. This is also
useful for analyzing multi-platform manifest lists to see what platforms are
available for a particular image.

### Layer Commands

The layer command acts on blobs within the registry. These blobs include the tar
layers and the json image configs.

```text
Usage:
  regctl layer [command]

Available Commands:
  pull        download a layer/blob
```

The `pull` command will pull a specific sha256 blob from the registry and
returns it to stdout. If you are requesting a tar layer, be sure to direct this
to a file. For json blobs, it's useful to redirect this to a command like `jq`.

Example usage:

```shell
$ regctl image manifest busybox
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
  "config": {
    "mediaType": "application/vnd.docker.container.image.v1+json",
    "size": 1493,
    "digest": "sha256:6858809bf669cc5da7cb6af83d0fae838284d12e1be0182f92f6bd96559873e3"
  },
  "layers": [
    {
      "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
      "size": 764618,
      "digest": "sha256:df8698476c65c2ee7ca0e9dbc2b1c8b1c91bce555819a9aaab724ac64241ba67"
    }
  ]
}

$ regctl layer pull busybox sha256:6858809bf669cc5da7cb6af83d0fae838284d12e1be0182f92f6bd96559873e3 | jq .
{
  "architecture": "amd64",
  "config": {
    "Hostname": "",
    "Domainname": "",
    "User": "",
    "AttachStdin": false,
    "AttachStdout": false,
    "AttachStderr": false,
    "Tty": false,
    "OpenStdin": false,
    "StdinOnce": false,
    ...
```

### Format Flag

The `--format` flag allows you to apply a Go template to the output of some
commands. The following functions have been added in addition to the defaults
available with Go:

- `json`: output the variable with json formatting
- `jsonPretty`: same as json with linefeeds and indentation
- `split`: split a string based on a separator
- `join`: append array entries into a string with a separator
- `title`: makes the first letter of each word uppercase
- `lower`: converts a string to lowercase
- `upper`: converts a string to uppercase

For more details, see:

- Go templates: <https://golang.org/pkg/text/template/>

Additionally for available fields, review the source for various types:

- OCI image spec: <https://github.com/opencontainers/image-spec/tree/master/specs-go/v1>
- Docker manifest: <https://github.com/docker/distribution/tree/master/manifest/schema2>
- Docker manifest list: <https://github.com/docker/distribution/tree/master/manifest/manifestlist>

Examples:

```shell
regctl image manifest --format '{{range .Layers}}{{println .Digest}}{{end}}' openjdk:latest # show each layer digest

regctl image inspect --format '{{jsonPretty .}}' alpine:latest # this is the default format

regctl image inspect --format '{{range $k, $v := .Config.Labels}}{{$k}} = {{$v}}{{println}}{{end}}' ... # loop through labels

regctl image inspect --format '{{range $k, $v := .Config.Labels}}{{if eq $k "org.label-schema.build-date"}}{{$v}}{{end}}{{end}}' ... # output a specific label
```
