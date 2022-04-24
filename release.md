# Release v0.4.2

Breaking Changes:

- Format templates no longer support `title`.
  ([PR 194][pr-194])

New Features:

- Add the ability to remove buildarg from image history.
  ([PR 207][pr-207])
- regsync now supports the `.Sync` template variable on source and target.
  ([PR 208][pr-208])
- Add support for annotations on docker2 schemas.
  ([PR 209][pr-209])
- Adding image signing with cosign.
  ([PR 212][pr-212])

Bug Fixes:

- Fix handling of http_proxy and https_proxy.
  ([PR 197][pr-197])
- manifest head on an ocidir with a digest no longer succeeds if manifest is missing.
  ([PR 199][pr-199])
- image export of manifest list to ocidir should only define parent manifest in `index.json`.
  ([PR 200][pr-200])
- Add `/tmp` directory to scratch images.
  ([PR 205][pr-205])
- Handle multiple tags pointing to the same digest in ocidir.
  ([PR 211][pr-211])

Other Changes:

- Upgrade to Go 1.17.
  ([PR 193][pr-193])
- Go build now runs with `-trimpath`.
  ([PR 196][pr-196])

[pr-193]: https://github.com/regclient/regclient/pull/193
[pr-194]: https://github.com/regclient/regclient/pull/194
[pr-196]: https://github.com/regclient/regclient/pull/196
[pr-197]: https://github.com/regclient/regclient/pull/197
[pr-199]: https://github.com/regclient/regclient/pull/199
[pr-200]: https://github.com/regclient/regclient/pull/200
[pr-205]: https://github.com/regclient/regclient/pull/205
[pr-207]: https://github.com/regclient/regclient/pull/207
[pr-208]: https://github.com/regclient/regclient/pull/208
[pr-209]: https://github.com/regclient/regclient/pull/209
[pr-211]: https://github.com/regclient/regclient/pull/211
[pr-212]: https://github.com/regclient/regclient/pull/212
