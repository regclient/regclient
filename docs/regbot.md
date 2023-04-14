# regbot Documentation

- [Top level commands](#top-level-commands)
- [Configuration file](#configuration-file)

## Top Level Commands

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
  version     Show the version

Flags:
  -c, --config string        Config file
      --dry-run              Dry Run, skip all external actions
  -h, --help                 help for regbot
      --logopt stringArray   Log options
  -v, --verbosity string     Log level (debug, info, warn, error, fatal, panic) (default "info")

Use "regbot [command] --help" for more information about a command.
```

The `once` command can be placed in a cron or CI job to perform the synchronization immediately rather than following the schedule.

The `server` command is useful to run a background process that continuously updates the target repositories as the source changes.

The `--dry-run` option is useful for testing scripts without actually copying or deleting images.

`--logopt` currently accepts `json` to format all logs as json instead of text.
This is useful for parsing in external tools like Elastic/Splunk.

The `version` command will show details about the git commit and tag if available.

## Configuration File

The `regbot` configuration file is yaml formatted with the following layout:

```yaml
x-sched-a: &sched-a "15 01 * * *"
version: 1
creds:
  - registry: localhost:5000
    tls: disabled
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

- `version`:
  This should be left at version 1 or not included at all.
  This may be incremented if future `regbot` releases change the configuration file structure.

- `creds`:
  Array of registry credentials and settings for connecting.
  To avoid saving credentials in the same file with the other settings, consider using the `${HOME}/.docker/config.json` or a template in the `user` and `pass`
  fields to expand a variable or file contents.
  When using the `ghcr.io/regclient/regbot` image, the docker config is read from `/home/appuser/.docker/config.json`.
  Each `creds` entry supports the following options:
  - `registry`:
    Hostname and port of the registry server used in image references.
    Use `docker.io` for Docker Hub.
    Note for parsing image names, a registry name must have a `.`, `:`, or be set to `localhost` to distinguish it from a path on Docker Hub.
  - `hostname`:
    Optional DNS name and port for the registry server, the default is the registry name.
    This allows multiple registry names to point to the same server with different configurations.
    This may be useful for different user logins, or different mirror configurations.
  - `user`:
    Username
  - `pass`:
    Password
  - `credHelper`:
    Name of a credential helper, typically in the form `docker-credential-name`.
    The alpine based docker image includes `docker-credential-ecr-login` and `docker-credential-gcr`.
  - `credExpire`:
    Duration to use a credential from a `credHelper`.
    This defaults to 1 hour.
    Use the [Go `time.Duration`](https://pkg.go.dev/time#ParseDuration) syntax when setting, e.g. `1h15m` or `30s`.
  - `tls`:
    Whether TLS is enabled/verified.
    Values include "enabled" (default), "insecure", or "disabled".
  - `regcert`:
    Registry CA certificate for self signed certificates.
    This may be a string with `\n` for line breaks, or the yaml multi-line syntax may be used like:

    ```yaml
    regcert: |
      -----BEGIN CERTIFICATE-----
      MIIJDDCCBPSgAwIB....
      -----END CERTIFICATE-----
    ```

  - `pathPrefix`:
    Path added before all images pulled from this registry.
    This is useful for some mirror configurations that place images under a specific path.
  - `mirrors`:
    Array of registry names to use as a mirror for this registry.
    Mirrors are sorted by priority, highest first.
    This registry is sorted after any listed mirrors with the same priority.
    Mirrors are not used for commands that change the registry, only for read commands.
  - `priority`:
    Non-negative integer priority used for sorting mirrors.
    This defaults to 0.
  - `repoAuth`:
    Configures authentication requests per repository instead of for the registry.
    This is required for some registry providers, specifically `gcr.io`.
    This defaults to `false`.
  - `blobChunk`:
    Chunk size for pushing blobs.
    Each chunk is a separate http request, incurring network overhead.
    The entire chunk is stored in memory, so chunks should be small enough not to exhaust RAM.
  - `blobMax`:
    Blob size which skips the single put request in favor of the chunked upload.
    Note that a failed blob put will fall back to a chunked upload in most cases.
    Disable with -1 to always try a single put regardless of blob size.
  - `reqPerSec`:
    Requests per second to throttle API calls to the registry.
    This may be a decimal like 0.5 to limit to one request every 2 seconds.
    Disable by leaving undefined or setting to 0.
  - `reqConcurrent`:
    Number of concurrent requests that can be made to the registry.
    Disable by leaving undefined or setting to 0.

- `defaults`:
  Global settings and default values applied to each sync entry:
  - `interval`:
    How often to run each sync step in `server` mode.
  - `schedule`:
    Cron like schedule to run each step, overrides `interval`.
  - `parallel`:
    Number of concurrent actions to run.
    All scripts may be started concurrently, but will wait on this limit when specific actions are performed like an image copy.
    Defaults to 1.
  - `timeout`:
    Time until the script is aborted.
    This timeout is enforced when calling various actions like an image copy.
  - `skipDockerConfig`:
    Do not read the user credentials in `${HOME}/.docker/config.json`.
  - `userAgent`:
    Override the user-agent for http requests.

- `scripts`:
  Array of Lua scripts to run.
  - `script`:
    Text of the Lua script.
  - `interval`, `schedule`, and `timeout`:
    See description under `defaults`.

- `x-*`:
  Any field beginning with `x-` is considered a user extension and will not be parsed in current for future versions of the project.
  These are useful for integrating your own tooling, or setting values for yaml anchors and aliases.

[Go templates](https://golang.org/pkg/text/template/) are used to expand values in `user`, `pass`, and `regcert`.
See [Template Functions](README.md#template-functions) for more details on the custom functions available in templates.

The Lua script interface is based on Lua 5.1.
The [Lua manual is available online](https://www.lua.org/manual/5.1/index.html).
The following additional functions are available:

- `log <msg>`:
  Log a message (preferred over Lua's print).
- `reference.new <ref>`:
  Accepts an image reference, returning a reference object.
  Other functions that accept an image name or repository will accept a reference object.
- `<ref>:digest`:
  Get or set the digest on a reference.
- `<ref>:tag`:
  Get or set the tag on a reference.
  This is useful when iterating over tags within a repository.
- `repo.ls <host:port> [opts]`:
  List the repositories on a registry server.
  This depends on the registry supporting the API call.
  Opts is a table that can have the following values set:
  - `limit`: number of results to return
  - `last`: last received repo, next batch of results will start after this

  e.g. `list = repo.ls("example.com", {limit = 500})`
- `tag.ls <repo>`:
  Returns an array of tags found within a repository.
- `tag.delete <ref>`:
  Deletes a tag from a registry.
  This uses the regclient tag delete method that first pushes a dummy manifest to the tag, which avoids deleting other tags that point to the same manifest.
- `manifest.get`:
  Returns the image manifest.
  The current platform will be resolved, or it may be specified as a second arg.
- `manifest.getList`:
  Retrieves a manifest list without resolving the current platform.
  If the manifest is not a multi-platform manifest list, the single manifest will be returned instead.
- `manifest.head`:
  Retrieves the manifest using a head request.
  This pulls the digest and current rate limit and can be used with the manifest delete and ratelimit functions.
- `manifest.put <manifest> <ref>`:
  Pushes a manifest to the provided reference.
- `<manifest>:config`:
  See `image.config`
- `<manifest>:delete`:
  Deletes a manifest.
  Note that a manifest list or manifest head request to retrieve the manifest is recommended, otherwise the registry may delete a single platform's manifest without deleting the entire multi-platform image, leading to errors when attempting to access the remaining manifest.
  If multiple tags can point to the same manifest, then using `tag.delete` is recommended.
- `<manifest>:export`:
  Returns a new manifest created with user changes to the current manifest data (user changes are ignored by all other calls).
- `<manifest>:get`:
  See `image.manifest`.
  This is useful for pulling a manifest when you've only run a head request.
- `<manifest>:put <ref>`:
  See `manifest.put`
- `<manifest>:ratelimit`:
  Return the ratelimit seen when the manifest was last retrieved.
  The ratelimit object includes `Set` (boolean indicating if a rate limit was returned with the manifest), `Remain` (requests remaining), `Limit`
  (maximum limit possible).
- `<manifest>:ratelimitWait <limit> <poll> <timeout>`:
  See `image.ratelimitWait`
- `blob.get <ref> <optional digest>`:
  Retrieve a blob from the repository in the reference.
  If a separate digest is not provided, the reference must include a digest.
- `blob.head <ref> <optional digest>`:
  Same as `blob.get` but only performs a head request.
- `blob.put <ref> <content>`:
  Reference is used to lookup the repository where the blob is pushed.
  Content is a string, another blob, or a config object.
  The digest and size of the pushed blob are returned.
- `<blob>:put <content>`:
  See `blob.put`.
- `<config>:export`:
  Returns a new config created with user changes to the current config data (user changes are ignored by all other calls).
- `image.config <ref>`:
  Returns the image configuration, see `docker image inspect`.
- `image.copy <src-ref> <tgt-ref>`:
  Copies an image.
  This may be retagging within the same repository, copying between repositories, or copying between registries.
  There's an optional 3rd argument with a table of options:
  - `{digestTags = true}`: copies digest specific tags in addition to the manifests.
  - `{forceRecursive = true}`: forces a copy of all manifests and blobs even when the target parent manifest already exists.
- `image.exportTar <src-ref> <tar-filename>`:
  Exports an image from the registry to a tar file.
- `image.importTar <tgt-ref> <tar-filename>`:
  Imports an image from a tar file to the registry.
- `image.ratelimitWait <ref> <limit> <poll> <timeout>`:
  Polls a registry for the rate limit remaining to increase at or above the specified limit.
  By default the polling interval is `5m` and timeout is `6h`.
