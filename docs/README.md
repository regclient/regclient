# regclient Documentation

- [API](#regclient-api)
- [regctl](regctl.md)
- [regsync](regsync.md)
- [regbot](regbot.md)
- [Template Functions](#template-functions)
- [FAQ](#faq)

## regclient API

API docs are still needed

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
  See Go time package for more details on implemented functions:
  - `time.Now`:
    Returns current time object, e.g. `{{ $t := time.Now }}{{printf "%d%d%d" $t.Year $t.Month $t.Day}}`.
  - `time.Parse`:
    Parses string using layout into time object, e.g. `{{ $t := time.Parse "1970-12-31" "2020-06-07"}}`.
- `title`:
  Makes the first letter of each word uppercase.
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

2. Q: Can I delete images from Docker Hub?

   A: Officially, Hub does not support the manifest delete API, you'll get an access denied.
   For now, I'm waiting to see if Docker will provide support for distribution-spec API's or at least provide tokens that can be used to manage images.
   Experimentally, the API call has been added to take a token and run the tag delete against `hub.docker.com` instead of `docker.io`.
   To implement this, add the following section to the `~/.regctl/config.json`:

   ```json
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
