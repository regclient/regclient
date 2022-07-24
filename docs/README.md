# regclient Documentation

- [Project Specific Documentation](#project-specific-documentation)
- [Schemes](#schemes)
- [Template Functions](#template-functions)
- [FAQ](#faq)

## Project Specific Documentation

- [regclient API](https://pkg.go.dev/github.com/regclient/regclient)
- [regctl](regctl.md)
- [regsync](regsync.md)
- [regbot](regbot.md)

## Schemes

For referencing a repository or image, the following schemes are supported:

- Registry:
  These are represented with the traditional image syntax (`registry:port/namespace/repo:tag`).
  The Docker Hub defaults are applied, if an image is missing a registry (with either a `.`, `:`, or `localhost` in the field before the first `/`) then it defaults to `docker.io`.
  And if a repository on Docker Hub is not provided, `library` is assumed which is the default for all Docker official images.
  Examples include `alpine` which pulls `docker.io/library/alpine:latest` from Docker Hub, and `registry.example.com:5000/username/project@sha256:3f5754829e9747db418bd1a5a40f418b073ed863cba4d57aaeaefa08118c4743` which pulls a specific digest from the named registry.
- `ocidir://`:
  This implements an [OCI Layout](https://github.com/opencontainers/image-spec/blob/main/image-layout.md) to a local directory.
  Multiple tags may be pushed/pulled to the same directory, making it equivalent to a repository on a registry.
  Use `ocidir://name:tag` to refer to the `./name` directory and `ocidir:///tmp/name:tag` to refer to the `/tmp/name` directory (the third leading slash denotes an absolute path).

These schemes can be used anywhere an image is referenced.

## Template Functions

Go templates are used in multiple regclient based commands.
See the [golang documentation](https://golang.org/pkg/text/template/) for details on base functionality.
The following functions have been added in addition to the defaults available in Go:

- `default`:
  Provide a default value when input is empty, e.g. `{{ env "VAR" | default "undefined" }}`.
- `env`:
  Expands provided environment variable, e.g. `{{ env "USER" }}`.
- `file`:
  Outputs contents of the file, leading and trailing whitespace is removed.
- `join`:
  Append array entries into a string with a separator.
- `json`:
  Output the variable with json formatting.
- `jsonPretty`:
  Same as json with linefeeds and indentation.
- `lower`:
  Converts a string to lowercase.
- `printPretty`:
  Outputs a user readable view of the object when available, otherwise falling back to `jsonPretty` output.
  This is useful for manifest lists and tag lists.
- `split`:
  Split a string based on a separator.
- `time`:
  See [Go time package](https://pkg.go.dev/time) for more details on implemented functions:
  - `time.Now`:
    Returns current time object, e.g. `{{ $t := time.Now }}{{printf "%d%d%d" $t.Year $t.Month $t.Day}}`.
  - `time.Parse`:
    Parses string using layout into time object, e.g. `{{ $t := time.Parse "2006-01-02" "2020-06-07"}}`.
- `upper`:
  Converts a string to uppercase.

## FAQ

1. Q: After deleting tags and images on the registry, I'm still seeing disk space being used.

   A: Registries require garbage collection to run to cleanup untagged manifests and unused blobs.
   The [registry GC documentation](https://docs.docker.com/registry/garbage-collection/) includes more details and there are various [pull requests](https://github.com/distribution/distribution/pull/3195) that improve garbage collection.
   Note that I've seen data loss from this PR and still need to submit an example of the issue upstream, so test and backup before implementing yourself.
   With the PR-3195 applied, I use the following for GC of anything at least 1 hour old:

   ```shell
   docker exec registry /bin/registry garbage-collect \
     --delete-repositories \
     --delete-untagged \
     --modification-timeout 3600 \
     /etc/docker/registry/config.yml
   ```

1. Q: Can I delete images from Docker Hub?

   A: Officially, Hub does not support the manifest delete API, you'll get an access denied.
   For now, I'm waiting to see if Docker will provide support for distribution-spec API's or at least provide tokens that can be used to manage images.
   Experimentally, the API call has been added to take a token and run the tag delete against `hub.docker.com` instead of `docker.io`.
   To implement this, add the following section to the `~/.regctl/config.json`:

   ```jsonc
   {
     "hosts": {
       "hub.docker.com": {
         "api": "hub",
         "token": "..."
       }
     }
   }
   ```

   The value of `token` can be found by inspecting the cookies when logged into docker hub from your browser.
   With this, you can run `regctl tag del hub.docker.com/${repo}:${tag}`, for your repo and tag.
   Similar API and Token fields are available in regbot, but this remains experimental.
   Note that no other APIs are currently expected to work against `hub.docker.com`, query `docker.io` for the tag list and inspecting images.

1. Q: Can I use docker credential helpers?

   A: If your `$HOME/.docker/config.json` includes a section like the following:

   ```jsonc
   "credHelpers": {
      "gcr.io": "gcr", 
      "public.ecr.aws": "ecr-login", 
   }
   ```

   These will work with standalone binaries or with the alpine image variants.
   You will need to include the source for the credentials as a volume when running the image (e.g. `$HOME/.aws` and `$HOME/.config/gcloud`).
   The alpine image only includes `ecr-login` and `gcr` helpers.
   Note that the `gcloud` helper is not included since it results in a significant increase in the alpine image size (40M vs over 500M).
   Instead you can switch to `gcr` and copy your key to `$HOME/.config/gcloud/application_default_credentials.json`.
   For more details on the gcr helper, see <https://github.com/GoogleCloudPlatform/docker-credential-gcr>.

1. Q: Actions against multiple `gcr.io` repositories fail with authentication errors.

   A: Authentication on `gcr.io` does not handle multiple scopes like other registries do.
   This can be solved to limiting the authentication to a single since repository on those registries.
   Set the `repoAuth` flag to true in yaml configurations to enable this:

   ```yaml
   creds:
   - registry: gcr.io
     repoAuth: true
   ```

   For `regctl`, use `regctl registry set --repo-auth gcr.io`.
