# regctl Documentation

- [Top level commands](#top-level-commands)
- [Registry commands](#registry-commands)
- [Repo commands](#repo-commands)  
- [Tag commands](#tag-commands)
- [Image commands](#image-commands)
- [Blob commands](#blob-commands)
- [Index commands](#index-commands)
- [Artifact commands](#artifact-commands)
- [Format flag](#format-flag)

## Top Level Commands

```text
$ regctl --help
Utility for accessing docker registries
More details at https://github.com/regclient/regclient

Usage:
  regctl [command]

Available Commands:
  artifact    manage artifacts
  blob        manage image blobs/layers
  completion  Generate completion script
  help        Help about any command
  image       manage images
  manifest    manage manifests
  registry    manage registries
  repo        manage repositories
  tag         manage tags
  version     Show the version

Flags:
  -h, --help                 help for regctl
      --logopt stringArray   Log options
  -v, --verbosity string     Log level (debug, info, warn, error, fatal, panic) (default "warning")

Use "regctl [command] --help" for more information about a command.
```

`--logopt` currently accepts `json` to format all logs as json instead of text.
This is useful for parsing in external tools like Elastic/Splunk.

The `version` command will show details about the git commit and tag if available.

Shell completion is available with the completion command, e.g. for `bash`:

```bash
source <(regctl completion bash)
```

Instructions for other shells is available from `regctl completion --help`.

## Registry Commands

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

With docker installed and logged into the registry, these commands are typically not needed with the exception of configuring an insecure registry.
The `regctl` will import credentials from the docker logins stored in `$HOME/.docker/config.json` and trust certificates loaded in `/etc/docker/certs.d/$registry/*.crt`.
These commands are useful for running in an environment without docker to configure the `$HOME/.regctl/config.json` file.
One use case for that is to run `regctl` within an unpriviliged container in a CI pipeline.
With the `regclient/regctl` image, the docker configuration is pulled from `/home/appuser/.docker/config.json` by default.

Note that it is possible to configure multiple registry servers under a single name as a mirror with automatic failover.
This is useful for pulling content, but pushes will still be sent to the upstream registry server.
For example, to configure `mirror-build:5000` and `mirror-cluster:5000` as the first and second mirrors (respectively) for Docker Hub:

```text
regctl registry set --priority 10 mirror-build:5000
regctl registry set --priority  5 mirror-cluster:5000
regctl registry set --mirror mirror-build:5000 --mirror mirror-cluster:5000 docker.io
```

## Repo Commands

```text
Usage:
  regctl repo [command]

Available Commands:
  ls          list repositories in a registry
```

The `ls` command lists repositories within a registry server.
This may not be implemented by every registry server.
Notably missing from the supported list is Docker Hub.

## Tag Commands

```text
Usage:
  regctl tag [command]

Available Commands:
  delete      delete a tag in a repo
  ls          list tags in a repo
```

The `ls` command lists all tags within a repo.

The `delete` command will delete a single tag without impacting other tags or the underlying manifest which is useful if you are unsure if your image is used elsewhere and want to rely on the registry to cleanup untagged manifests.

## Image Commands

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
  ratelimit   show the current rate limit
```

The `copy` command allows images to be copied between registries, between repositories on the same registry, or retag an image within the same repository, and only pulls the layers when needed (typically not needed with the same registry server).

The `delete` command removes the image manifest from the server.
This will impact all tags pointing to the same manifest and requires a digest to be included in the image reference to be deleted (e.g. `myimage@sha256:abcd...`).
Using `--force-tag-dereference` will automatically lookup the digest for a specific tag, and will delete the underlying image which will delete any other tags pointing to the same image.
Use `tag delete` to remove a single tag.

The `digest` command is useful to pin the image used within your deployment to an immutable sha256 checksum.

The `export`/`import` commands allow you to copy images between registry servers that may be disconnected, or to export an image directly from a registry without a docker engine and loading it into a potentially disconnected docker host. (Note that import is not yet implemented.)

The `inspect` command pulls the image config json blob. This is the same json shown with a `docker image inspect` command, and includes labels, the entrypoint/cmd, and layer history.
This can be useful with image pruning scripts, or other tools that need the image labels without the need to pull all of the layers.

The `manifest` command shows the low level layers and digests that can be pulled from the registry to retrieve individual components of an image.
This is also useful for analyzing multi-platform manifest lists to see what platforms are available for a particular image.

The `ratelimit` command shows the current rate limit on the manifest API using a http HEAD request that does not count against the Docker Hub limits.

## Manifest Commands

The manifest command acts on manifests within the registry.
These manifests are the top level of an image, and many commands are aliases for `image` commands.

```text
Usage:
  regctl manifest [command]

Available Commands:
  delete      delete a manifest
  diff        compare manifests
  digest      retrieve digest of manifest
  get         retrieve manifest or manifest list
  put         push manifest or manifest list
```

The `delete` command removes the image manifest from the server.
This will impact all tags pointing to the same manifest and requires a digest to be included in the image reference to be deleted (e.g. `myimage@sha256:abcd...`).
Using `--force-tag-dereference` will automatically lookup the digest for a specific tag, and will delete the underlying image which will delete any other tags pointing to the same image.
Use `tag delete` to remove a single tag.

The `diff` command compares two manifests and shows what has changed between these manifests.
See also the `blob diff-config` and `blob diff-layer` commands.

The `digest` command is useful to pin the image used within your deployment to an immutable sha256 checksum.

The `get` command retrieves the manifest from the registry, showing individual components of an image.
This is also useful for analyzing multi-platform manifest lists to see what platforms are available for a particular image.

The `put` command uploads the manifest to the registry.
This can be used to create or modify an image.
The format option includes `.Manifest` which supports methods from [manifest.Manifest](https://pkg.go.dev/github.com/regclient/regclient/types/manifest#Manifest).

## Blob Commands

The layer command acts on blobs within the registry.
These blobs include the tar layers and the json image configs.

```text
Usage:
  regctl blob [command]

Available Commands:
  diff-config diff two image configs
  diff-layer  diff two tar layers
  get         download a blob/layer
  put         upload a blob/layer
```

The `diff-config` command compares two config blobs, showing the differences between the configs.

The `diff-layer` command compares two layer blobs, showing exactly what changed in the filesystem between the two layers.

Example usage:

```shell
$ regctl blob diff-layer --context 0 --ignore-timestamp \
    alpine sha256:627fad6f28f79c3907ad18a4399be4d810c0e1bb503fe3712217145c555b9d2f \
    alpine sha256:decfdc335d9bae9ca06166e1a4fc2cdf8c2344a42d85c8a1d3f964aab59ecff5
@@ -6,1 +6,1 @@
- -rwxr-xr-x 0/0   824904 bin/busybox                              sha256:4a1876b4899ce26853ec5f5eb75248e5a2d9e07369c4435c8d41e83393e04a9b
+ -rwxr-xr-x 0/0   829000 bin/busybox                              sha256:d15929a78a86065c41dd274f2f3f058986b6f5eee4a4c881c83d4fa4179e58ee
@@ -85,1 +85,1 @@
- -rw-r--r-- 0/0        8 etc/alpine-release                       sha256:9fa33d932bbf6e5784f15b467a9a10e4ce43993c2341ee742f23ce0196fd73e9
+ -rw-r--r-- 0/0        7 etc/alpine-release                       sha256:922fe0c3de073b01988e23348ea184456161678c5e329e6f34be89be24383f93
@@ -95,1 +95,1 @@
- -rw-r--r-- 0/0      103 etc/apk/repositories                     sha256:e44b25ef011171afece2ff51a206b732f84c7f3ddc8291c6dc50cb1572c0ae1c
+ -rw-r--r-- 0/0      103 etc/apk/repositories                     sha256:7b5dba82c50baee0b4aee54038ca2265df42d1f873d1601934bb45daf17311b4
@@ -101,1 +101,1 @@
- -rw-r--r-- 0/0      682 etc/group                                sha256:412af628e00706d3c90a5d465d59cc422ff68d79eeb8870c4f33ed6df04b2871
+ -rw-r--r-- 0/0      697 etc/group                                sha256:0632d55a68081065097472fe7bc7c66f0785f3b78f39fb23f622d24a7e09be9f
@@ -106,1 +106,1 @@
...
```

The `get` command will pull a specific sha256 blob from the registry and returns it to stdout.
If you are requesting a tar layer, be sure to direct this to a file or command that parses the content.
For json blobs, it's useful to redirect this to a command like `jq`.

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

$ regctl blob get busybox sha256:6858809bf669cc5da7cb6af83d0fae838284d12e1be0182f92f6bd96559873e3 | jq .
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

The `put` command uploads a blob to the registry.
The digest of the blob is output.
Note that blobs should be referenced by a manifest to avoid garbage collection.

The `--format` option to `put` has the following variables available:

- `.Digest`: digest of the pushed blob
- `.Size`: size of the pushed blob

## Index Commands

The index command creates or manages OCI Indexes and Manifest Lists.

```text
Usage:
  regctl index [command]

Available Commands:
  add         add an index entry
  create      create an index
  delete      delete an index entry
```

The `create` command is used to create a new Index and optionally include an initial set of manifests.
The `add` and `delete` commands are used to add and remove manifests from the Index.
When adding manifests to an Index, references in other repositories will first be copied to the local repository.
The platform will automatically be added when an image has a config containing those fields.

## Artifact Commands

The artifact command works with OCI artifacts.
This is used to store arbitrary data within a registry, with an associated manifest and json config.

```text
Usage:
  regctl artifact [command]

Available Commands:
  get         download artifacts
  put         upload artifacts
```

The `get` command retrieves an artifact from the registry.
By default, the artifact contents are written to stdout, redirect this to a file for binary content.
For retrieving multiple files from a single artifact, specify an output directory.
Filters can be added for the filename and media type, and the config json can also be output to a separate file.

The `put` command uploads an artifact to the registry.
Each file should have a media type passed in the same order on the command line.
A single file may be pushed using stdin.
The config json may also be pushed, and have it's own media type.
To set annotations on the manifest, use `--annotation name=value`, and repeat the flag for additional annotations.
The format option includes `.Manifest` which supports methods from [manifest.Manifest](https://pkg.go.dev/github.com/regclient/regclient/types/manifest#Manifest).

The following demonstrates uploading a simple artifact from stdin/stdout:

```shell
$ regctl artifact put \
  --annotation demo=true --annotation format=oci \
  --format '{{ .Manifest.GetDescriptor.Digest }}' \
  localhost:5000/artifact:demo <<EOF
Test artifact from regctl.
This follows the OCI artifact format
EOF
sha256:36484d44383fc9ffd34be11da4a617a96cb06b912c98114bfdb6ad2dddd443e2

$ regctl manifest get localhost:5000/artifact:demo
Name:        localhost:5000/artifact:demo
MediaType:   application/vnd.oci.image.manifest.v1+json
Digest:      sha256:36484d44383fc9ffd34be11da4a617a96cb06b912c98114bfdb6ad2dddd443e2
Annotations: 
  demo:      true
  format:    oci
Total Size:  64B
             
Config:      
  Digest:    sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a
  MediaType: application/vnd.unknown.config.v1+json
  Size:      2B
             
Layers:      
             
  Digest:    sha256:7f8028bb058b780630dcd31cde93cb3efe96d60108ffbfe2727e4e76fdf4c9dc
  MediaType: application/octet-stream
  Size:      64B

$ regctl artifact get localhost:5000/artifact:demo
Test artifact from regctl.
This follows the OCI artifact format
```

## Format Flag

The `--format` flag allows you to apply a Go template to the output of some commands.
For more details of Go templates, see:

- Go templates: <https://golang.org/pkg/text/template/>

For a list of added template functions, see [Template Functions](README.md#template-functions)

Additionally for available fields, review the source for various types:

- OCI image spec: <https://github.com/opencontainers/image-spec/tree/master/specs-go/v1>
- Docker manifest: <https://github.com/docker/distribution/tree/master/manifest/schema2>
- Docker manifest list: <https://github.com/docker/distribution/tree/master/manifest/manifestlist>

Several commands expand the following format strings:

- `raw`: this returns the raw headers and body.
- `rawBody`, `raw-body`, or `body`: this returns the original body of the response.
- `rawHeaders`, `raw-headers`, or `headers`: this returns the full HTTP headers of the response.

Examples:

```shell
regctl image manifest --format '{{range .Layers}}{{println .Digest}}{{end}}' openjdk:latest # show each layer digest

regctl image inspect --format '{{jsonPretty .}}' alpine:latest

regctl image inspect --format '{{range $k, $v := .Config.Labels}}{{$k}} = {{$v}}{{println}}{{end}}' ... # loop through labels

regctl image inspect --format '{{range $k, $v := .Config.Labels}}{{if eq $k "org.label-schema.build-date"}}{{$v}}{{end}}{{end}}' ... # output a specific label

regctl image manifest --format raw-body alpine:latest # returns the raw manifest
```
