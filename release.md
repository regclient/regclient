# Release v0.4.5

Breaking Changes:

- `regclient.ManifestWithDesc` has been renamed to `regclient.WithManifestDesc` ([PR 283][pr-283])
- `manifest.Referrer` interface has been renamed to `manifest.Subjecter` ([PR 283][pr-283], [PR 302][pr-302])
- `ReferrerPut` method has been removed, use `ManifestPut` instead ([PR 283][pr-283])
- `regclient.ManifestPut` options are now in `regclient` rather than scheme ([PR 283][pr-283])

New Features:

- Adding diff commands to regctl for manifests, configs, and layers ([PR 269][pr-269])
- Supporting OCI subject/referrers ([PR 271][pr-271], [PR 302][pr-302])
- Adding support for client side filters on artifact list ([PR 273][pr-273])
- Support Link header in tag pagination seen on quay.io ([PR 276][pr-276])
- Update referrers to support deletes ([PR 283][pr-283])
- Add `regctl image check-base` ([PR 288][pr-288])
- Adding experimental support for rebasing images ([PR 291][pr-291])
- regclient adds a blob TarReader.ReadFile method ([PR 296][pr-296])
- Adding `regctl blob get-file` to fetch a file from a layer ([PR 296][pr-296])
- Adding `regctl image get-file` to fetch a file from the image layers ([PR 296][pr-296])
- Fallback to using linux platforms on mac and windows when querying manifest lists ([PR 303][pr-303])

Bug Fixes:

- Fix reference for ocidir exports for importing into docker ([PR 264][pr-264])
- Fix support for authentication parsing of token characters without quotes ([PR 266][pr-266])
- Fix `regctl artifact put` with both a refers and a tag ([PR 290][pr-290])
- Fixing regbot tests to run on other platforms ([PR 300][pr-300])

Other Changes:

- Upgrade to Go 1.19 ([PR 278][pr-278], [PR 286][pr-286])
- Add tips to common errors in regctl ([PR 285][pr-285])
- Descriptor comparison now allows for Docker to OCI conversion ([PR 289][pr-289])

[pr-264]: https://github.com/regclient/regclient/pull/264
[pr-266]: https://github.com/regclient/regclient/pull/266
[pr-269]: https://github.com/regclient/regclient/pull/269
[pr-271]: https://github.com/regclient/regclient/pull/271
[pr-273]: https://github.com/regclient/regclient/pull/273
[pr-276]: https://github.com/regclient/regclient/pull/276
[pr-278]: https://github.com/regclient/regclient/pull/278
[pr-283]: https://github.com/regclient/regclient/pull/283
[pr-285]: https://github.com/regclient/regclient/pull/285
[pr-286]: https://github.com/regclient/regclient/pull/286
[pr-288]: https://github.com/regclient/regclient/pull/288
[pr-289]: https://github.com/regclient/regclient/pull/289
[pr-290]: https://github.com/regclient/regclient/pull/290
[pr-291]: https://github.com/regclient/regclient/pull/291
[pr-296]: https://github.com/regclient/regclient/pull/296
[pr-300]: https://github.com/regclient/regclient/pull/300
[pr-302]: https://github.com/regclient/regclient/pull/302
[pr-303]: https://github.com/regclient/regclient/pull/303
