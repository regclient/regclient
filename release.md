# Release v0.5.1

Features:

- Add options to `regctl index create` for `artifactType` and `subject` ([PR 490][pr-490])
- Add `--latest` flag to `regctl artifact get` and `regctl artifact list` ([PR 507][pr-507])
- Add in memory caching support. ([PR 510][pr-510])

Fixes:

- Fix typo in OCI annotations ([PR 485][pr-485])
- Support multiple dashes in repo names. ([PR 489][pr-489])
- Fix auth failures, do not trigger a backoff ([PR 492][pr-492])
- Update Go dependencies from all subdirectories and used in tests ([PR 495][pr-495])
- Warning header log message is fixed. ([PR 497][pr-497])
- Include version annotation in regclient images ([PR 499][pr-499])
- Fix digest check when running `regctl image get-file` ([PR 503][pr-503])
- Switch `org.opencontainers.artifact.*` to `org.opencontainers.image.*` annotations in regclient images. ([PR 506][pr-506])

Chores:

- Sync OCI types to align with upstream sources. ([PR 488][pr-488])
- Add OSV vulnerability scanner ([PR 498][pr-498])

[pr-485]: https://github.com/regclient/regclient/pull/485
[pr-488]: https://github.com/regclient/regclient/pull/488
[pr-489]: https://github.com/regclient/regclient/pull/489
[pr-490]: https://github.com/regclient/regclient/pull/490
[pr-492]: https://github.com/regclient/regclient/pull/492
[pr-495]: https://github.com/regclient/regclient/pull/495
[pr-497]: https://github.com/regclient/regclient/pull/497
[pr-498]: https://github.com/regclient/regclient/pull/498
[pr-499]: https://github.com/regclient/regclient/pull/499
[pr-503]: https://github.com/regclient/regclient/pull/503
[pr-506]: https://github.com/regclient/regclient/pull/506
[pr-507]: https://github.com/regclient/regclient/pull/507
[pr-510]: https://github.com/regclient/regclient/pull/510
