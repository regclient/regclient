# Release v0.4.0

Breaking Changes:

- Repository has been restructured to remove the `regclient` sub-directory.
  Backwards compatible aliases and stub functions have been added to minimize the impact.
  ([PR 130][pr-130])
- Refactored the manifest type. This results in breaking changes to exported methods.
  ([PR 157][pr-157])
- Blob methods have been refactored with breaking changes to exported APIs.
  ([PR 158][pr-158])
- External dependencies have been minimized, particularly for struct definitions.
  This impacts variable types used by some APIs.
  ([PR 160][pr-160])
- Default to displaying manifest lists in regctl.
  ([PR 166][pr-166])
- Image manifests now display with a pretty printer by default.
  ([PR 168][pr-168])

New Features:

- APIs have been updated to support the `ocidir://` scheme.
  You can now `regctl image copy alpine:latest ocidir://alpine:latest` to copy an image to a directory as you would another registry.
  ([PR 138][pr-138], [PR 146][pr-146])
- Added the ability to modify a manifest type.
  ([PR 157][pr-157])
- Blob SetConfig method added to allow modifying an OCI config.
  ([PR 158][pr-158])
- Added repoAuth option for gcr.io support.
  ([PR 159][pr-159])

Bug Fixes:

- Lots of linting and testing has been added.
  This will result in a change to some error messages.
  ([PR 146][pr-146], [PR 147][pr-147], [PR 151][pr-151])
- Fix handling of docker logins where scheme is included in the hostname.
  ([PR 137][pr-137])
- Fix for tag rm with authentication when the blob upload location is a different host.
  ([PR 144][pr-144])

[pr-130]: https://github.com/regclient/regclient/pull/130
[pr-137]: https://github.com/regclient/regclient/pull/137
[pr-138]: https://github.com/regclient/regclient/pull/138
[pr-144]: https://github.com/regclient/regclient/pull/144
[pr-146]: https://github.com/regclient/regclient/pull/146
[pr-147]: https://github.com/regclient/regclient/pull/147
[pr-151]: https://github.com/regclient/regclient/pull/151
[pr-157]: https://github.com/regclient/regclient/pull/157
[pr-158]: https://github.com/regclient/regclient/pull/158
[pr-159]: https://github.com/regclient/regclient/pull/159
[pr-160]: https://github.com/regclient/regclient/pull/160
[pr-166]: https://github.com/regclient/regclient/pull/166
[pr-168]: https://github.com/regclient/regclient/pull/168
