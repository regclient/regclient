# RegClient Documentation

## regclient

API docs are still needed

## regctl

### regctl Top level commands

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
  -h, --help                 help for regctl
      --logopt stringArray   Log options
  -v, --verbosity string     Log level (debug, info, warn, error, fatal, panic) (default "warning")

Use "regctl [command] --help" for more information about a command.
```

`--logopt` currently accepts `json` to format all logs as json instead of text.
This is useful for parsing in external tools like Elastic/Splunk.

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
CI pipeline. With the `regclient/regctl` image, the docker configuration is
pulled from `/home/appuser/.docker/config.json` by default.

Note that it is possible to configure multiple registry servers under a single
name as a mirror with automatic failover. This is useful for pulling content,
but may have unexpected results when pushing changes to the registry since each
http request will be sent to the first server in the list that is currently
available.

### Repo commands

```text
Usage:
  regctl repo [command]

Available Commands:
  ls          list repositories in a registry
```

The `ls` command lists repositories within a registry server. This may not be
implemented by every registry server. Notably missing from the supported list is
Docker Hub.

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
  ratelimit   show the current rate limit
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

The `ratelimit` command shows the current rate limit on the manifest API using a
http HEAD request that does not count against the Docker Hub limits.

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
commands. For more details of Go templates, see:

- Go templates: <https://golang.org/pkg/text/template/>

For a list of added template functions, see [Template Functions](#Template-Functions)

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

## regsync

### regsync Top Level Commands

```text
$ regsync --help
Utility for mirroring docker repositories
More details at https://github.com/regclient/regclient

Usage:
  regsync [command]

Available Commands:
  check       processes each sync command once but skip actual copy
  help        Help about any command
  once        processes each sync command once, ignoring cron schedule
  server      run the regsync server

Flags:
  -c, --config string        Config file
  -h, --help                 help for regsync
      --logopt stringArray   Log options
  -v, --verbosity string     Log level (debug, info, warn, error, fatal, panic) (default "info")
```

The `check` command is useful for reporting any stale images that need to be
updated.

The `once` command can be placed in a cron or CI job to perform the
synchronization immediately rather than following the schedule.

The `server` command is useful to run a background process that continuously
updates the target repositories as the source changes.

`--logopt` currently accepts `json` to format all logs as json instead of text.
This is useful for parsing in external tools like Elastic/Splunk.

### regsync Configuration File

The `regsync` configuration file is yaml formatted with the following layout:

```yaml
x-sched-a: &sched-a "15 01 * * *"
version: 1
creds:
  - registry: localhost:5000
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
sync:
  - source: busybox:latest
    target: localhost:5000/library/busybox:latest
    type: image
    interval: 60m
    backup: "backup-{{.Ref.Tag}}"
  - source: alpine
    target: localhost:5000/library/alpine
    type: repository
    tags:
      allow:
      - "latest"
      - "edge"
      - "3"
      - "3.\\d+"
      deny:
      - "3.0"
    schedule: *sched-a
    backup: "{{$t := time.Now}}{{printf \"%s/backups/%s:%s-%d%d%d\" .Ref.Registry .Ref.Repository .Ref.Tag $t.Year $t.Month $t.Day}}"
```

- `version`: This should be left at version 1 or not included at all. This may
  be incremented if future `regsync` releases change the configuration file
  structure.

- `creds`: Array of registry credentials and settings for connecting. To avoid
  saving credentials in the same file with the other settings, consider using
  the `${HOME}/.docker/config.json` or a template in the `user` and `pass`
  fields to expand a variable or file contents. When using the
  `regclient/regsync` image, the docker config is read from
  `/home/appuser/.docker/config.json`. Each `creds` entry supports the following
  options:
  - `registry`: Hostname and port of the registry server used in later lines. Use `docker.io` for Docker Hub.
  - `user`: Username
  - `pass`: Password
  - `tls`: Whether TLS is enabled/verified. Values include "enabled" (default), "insecure", or "disabled".
  - `scheme`: http or https (default)
  - `regcert`: Registry CA certificate for self signed certificates. This may be a string with `\n` for line breaks, or the yaml multi-line syntax may be used like:

    ```yaml
    regcert: |
      -----BEGIN CERTIFICATE-----
      MIIJDDCCBPSgAwIB....
      -----END CERTIFICATE-----
    ```

- `defaults`: Global settings and default values applied to each sync entry:
  - `backup`: Tag or image reference for backing up target image before
    overwriting. This may include a Go template syntax. This backup is only run
    when the source changes and the target exists that is about to be
    overwritten. If the backup tag already exists, it will be overwritten.
  - `interval`: How often to run each sync step in `server` mode.
  - `schedule`: Cron like schedule to run each step, overrides `interval`.
  - `ratelimit`: Settings to throttle based on source rate limits.
    - `min`: Minimum number of pulls remaining to start the step. Actions while
      running the step can result in going below this limit. Note that parallel
      steps and multi-platform images may each result in more than one pull
      happening beyond this threshold.
    - `retry`: How long to wait before checking if the rate limit has increased.
  - `parallel`: Number of concurrent image copies to run. All sync steps may be
    started concurrently to check if a mirror is needed, but will wait on this
    limit when a copy is needed. Defaults to 1.
  - `skipDockerConfig`: Do not read the user credentials in
    `${HOME}/.docker/config.json`.

- `sync`: Array of steps to run for copying images from the source to target
  repository.
  - `source`: Source image or repository.
  - `target`: Target image or repository.
  - `type`: "repository" or "image". Repository will copy all tags from the
    source repository.
  - `tags`: implements filters on tags for "repository" types, regex values are automatically bound to the beginning and ending of each string (`^` and `$`).
    - `allow`: array of regex strings to allow specific tags.
    - `deny`: array of regex strings to deny specific tags.
  - `platform`: single platform to pull from a multi-platform image, e.g.
    `linux/amd64`. By default all platforms are copied along with the original
    upstream manifest list. Note that looking up the platform from a
    multi-platform image counts against the Docker Hub rate limit, and that rate
    limits are not checked prior to resolving the platform. When run with
    "server", the platform is only resolved once for each multi-platform digest
    seen.
  - `backup`, `interval`, `schedule`, and `ratelimit`: See description under `defaults`.

- `x-*`: Any field beginning with `x-` is considered a user extension and will
  not be parsed in current for future versions of the project. These are useful
  for integrating your own tooling, or setting values for yaml anchors and
  aliases.

[Go templates](https://golang.org/pkg/text/template/) are used to expand values
in `user`, `pass`, `regcert`, `source`, `target`, and `backup`. The `backup`
template supports the following objects:

- `.Ref`: reference object about to be overwritten
  - `.Ref.Reference`: full reference
  - `.Ref.Registry`: registry name
  - `.Ref.Repository`: repository
  - `.Ref.Tag`: tag
- `.Step`: values from the current step
  - `.Step.Source`: source
  - `.Step.Target`: target
  - `.Step.Type`: type
  - `.Step.Backup`: backup
  - `.Step.Interval`: interval
  - `.Step.Schedule`: schedule

See [Template Functions](#Template-Functions) for more details on the custom
functions available in templates.

## regbot

### regbot Top Level Commands

```text
$ regbot --help
Utility for automating repository actions
More details at https://github.com/regclient/regclient

Usage:
  regbot [command]

Available Commands:
  help        Help about any command
  once        runs each script once
  server      run the regbot server

Flags:
  -c, --config string        Config file
      --dry-run              Dry Run, skip all external actions
  -h, --help                 help for regbot
      --logopt stringArray   Log options
  -v, --verbosity string     Log level (debug, info, warn, error, fatal, panic) (default "info")

Use "regbot [command] --help" for more information about a command.
```

The `once` command can be placed in a cron or CI job to perform the
synchronization immediately rather than following the schedule.

The `server` command is useful to run a background process that continuously
updates the target repositories as the source changes.

The `--dry-run` option is useful for testing scripts without actually copying or
deleting images.

`--logopt` currently accepts `json` to format all logs as json instead of text.
This is useful for parsing in external tools like Elastic/Splunk.

### regbot Configuration File

The `regbot` configuration file is yaml formatted with the following layout:

```yaml
x-sched-a: &sched-a "15 01 * * *"
version: 1
creds:
  - registry: localhost:5000
    tls: disabled
    scheme: http
  - registry: docker.io
    user: "{{env \"HUB_USER\"}}"
    pass: "{{file \"/var/run/secrets/hub_token\"}}"
defaults:
  parallel: 2
  timeout: 300s
scripts:
  - name: Hello World
    timeout: 10s
    script: |
      log "hello world"
  - name: Busybox Tags
    timeout: 30s
    script: |
      tags = tag.ls("busybox")
      table.sort(tags)
      for k, t in ipairs(tags) do
        log "Found tag " .. t
      end
```

- `version`: This should be left at version 1 or not included at all. This may
  be incremented if future `regbot` releases change the configuration file
  structure.

- `creds`: Array of registry credentials and settings for connecting. To avoid
  saving credentials in the same file with the other settings, consider using
  the `${HOME}/.docker/config.json` or a template in the `user` and `pass`
  fields to expand a variable or file contents. When using the
  `regclient/regsync` image, the docker config is read from
  `/home/appuser/.docker/config.json`. Each `creds` entry supports the following
  options:
  - `registry`: Hostname and port of the registry server used in later lines. Use `docker.io` for Docker Hub.
  - `user`: Username
  - `pass`: Password
  - `tls`: Whether TLS is enabled/verified. Values include "enabled" (default), "insecure", or "disabled".
  - `scheme`: http or https (default)
  - `regcert`: Registry CA certificate for self signed certificates. This may be a string with `\n` for line breaks, or the yaml multi-line syntax may be used like:

    ```yaml
    regcert: |
      -----BEGIN CERTIFICATE-----
      MIIJDDCCBPSgAwIB....
      -----END CERTIFICATE-----
    ```

- `defaults`: Global settings and default values applied to each sync entry:
  - `interval`: How often to run each sync step in `server` mode.
  - `schedule`: Cron like schedule to run each step, overrides `interval`.
  - `parallel`: Number of concurrent actions to run. All scripts may be started
    concurrently, but will wait on this limit when specific actions are
    performed like an image copy. Defaults to 1.
  - `timeout`: Time until the script is aborted. This timeout is enforced when
    calling various actions like an image copy.
  - `skipDockerConfig`: Do not read the user credentials in
    `${HOME}/.docker/config.json`.

- `scripts`: Array of Lua scripts to run.
  - `script`: Text of the Lua script.
  - `interval`, `schedule`, and `timeout`: See description under `defaults`.

- `x-*`: Any field beginning with `x-` is considered a user extension and will
  not be parsed in current for future versions of the project. These are useful
  for integrating your own tooling, or setting values for yaml anchors and
  aliases.

[Go templates](https://golang.org/pkg/text/template/) are used to expand values
in `user`, `pass`, and `regcert`. See [Template Functions](#Template-Functions)
for more details on the custom functions available in templates.

The Lua script interface is based on Lua 5.1. The [Lua manual is available
online](https://www.lua.org/manual/5.1/index.html). The following additional
functions are available:

- `log <msg>`: log a message (preferred over Lua's print).
- `reference.new <ref>`: accepts an image reference, returning a reference
  object. Other functions that accept an image name or repository will accept a
  reference object.
- `<ref>:tag`: get or set the tag on a reference. This is useful when iterating
  over tags within a repository.
- `repo.ls <host:port>`: list the repositories on a registry server. This
  depends on the registry supporting the API call.
- `tag.ls <repo>`: returns an array of tags found within a repository.
- `tag.delete <ref>`: deletes a tag from a registry. This uses the regclient tag
  delete method that first pushes a dummy manifest to the tag, which avoids
  deleting other tags that point to the same manifest.
- `manifest.get`: returns the image manifest. The current platform will
  be resolved, or it may be specified as a second arg.
- `manifest.getList`: retrieves a manifest list without resolving the
  current platform. If the manifest is not a multi-platform manifest list, the
  single manifest will be returned instead.
- `manifest.head`: retrieves the manifest using a head request. This
  pulls the digest and current rate limit and can be used with the manifest
  delete and ratelimit functions.
- `<manifest>:config`: see `image.config`
- `<manifest>:delete`: deletes a manifest. Note that a manifest list or manifest
  head request to retrieve the manifest is recommended, otherwise the registry
  may delete a single platform's manifest without deleting the entire
  multi-platform image, leading to errors when attempting to access the
  remaining manifest. If multiple tags can point to the same manifest, then
  using `tag.delete` is recommended.
- `<manifest>:get`: see `image.manifest`. This is useful for pulling a manifest
  when you've only run a head request.
- `<manifest>:ratelimit`: return the ratelimit seen when the manifest was last
  retrieved. The ratelimit object includes `Set` (boolean indicating if a rate
  limit was returned with the manifest), `Remain` (requests remaining), `Limit`
  (maximum limit possible).
- `<manifest>:ratelimitWait <limit> <poll> <timeout>`: see `image.ratelimitWait`
- `image.config <ref>`: returns the image configuration, see `docker image
  inspect`.
- `image.copy <src-ref> <tgt-ref>`: copies an image. This may be retagging
  within the same repository, copying between repositories, or copying between
  registries.
- `image.ratelimitWait <ref> <limit> <poll> <timeout>`: polls a registry for the
  rate limit remaining to increase at or above the specified limit. By default
  the polling interval is `5m` and timeout is `6h`.

## Template Functions

The following functions have been added in addition to the defaults
available with Go:

- `default`: provide a default value when input is empty, e.g. `{{ env "VAR" |
  default "undefined" }}`
- `env`: expands provided environment variable, e.g. `{{ env "USER" }}`
- `file`: outputs contents of the file, leading and trailing whitespace is
  removed
- `join`: append array entries into a string with a separator
- `json`: output the variable with json formatting
- `jsonPretty`: same as json with linefeeds and indentation
- `lower`: converts a string to lowercase
- `split`: split a string based on a separator
- `time`: see Go time package for more details on implemented functions
  - `time.Now`: returns current time object, e.g. `{{ $t := time.Now }}{{printf
    "%d%d%d" $t.Year $t.Month $t.Day}}`
  - `time.Parse`: parses string using layout into time object, e.g. `{{ $t :=
    time.Parse "1970-12-31" "2020-06-07"}}`
- `title`: makes the first letter of each word uppercase
- `upper`: converts a string to uppercase

## FAQ

1. Q: After deleting tags and images on the registry, I'm still seeing disk space being used.

   A: Registries require garbage collection to run to cleanup untagged manifests and unused blobs. The [registry GC documentation](https://docs.docker.com/registry/garbage-collection/) includes more details and there are various [pull requests](https://github.com/distribution/distribution/pull/3195) that improve garbage collection. With the PR-3195 applied, I use the following for GC of anything at least 1 hour old:

   ```shell
   docker exec registry /bin/registry garbage-collect \
     --delete-repositories \
     --delete-untagged \
     --modification-timeout 3600 \
     /etc/docker/registry/config.yml
   ```
