# Release v0.5.7

Changes:

- Add a `--skip-check` option to `regctl registry set` and `regctl registry login`. ([PR 646][pr-646])

Fixes:

- Improve error handling on blob put retries when source is not an io.Seeker. ([PR 622][pr-622])
- Preserve descriptor contents on chunked blob push. (@edigaryev) ([PR 637][pr-637])
- Validate descriptor contents on chunked blob push. ([PR 637][pr-637])

Chores:

- Improve testing to detect race conditions in registry operations. ([PR 634][pr-634])
- Update `ImageCopy` test to not depend on `ImageCopy` for setup. ([PR 635][pr-635])
- leverage `t.Setenv` in tests of environment variables. ([PR 636][pr-636])
- reduce logging of context canceled messages in an image copy failure. ([PR 639][pr-639])
- Add tests for TagList and TagDelete. ([PR 640][pr-640])
- Upgrade olareg testing harness to latest version. ([PR 648][pr-648])
- Update OSV scanner to use new syntax. ([PR 652][pr-652])

Contributors:

- @sudo-bmitch

[pr-634]: https://github.com/regclient/regclient/pull/634
[pr-635]: https://github.com/regclient/regclient/pull/635
[pr-636]: https://github.com/regclient/regclient/pull/636
[pr-622]: https://github.com/regclient/regclient/pull/622
[pr-637]: https://github.com/regclient/regclient/pull/637
[pr-639]: https://github.com/regclient/regclient/pull/639
[pr-640]: https://github.com/regclient/regclient/pull/640
[pr-646]: https://github.com/regclient/regclient/pull/646
[pr-648]: https://github.com/regclient/regclient/pull/648
[pr-652]: https://github.com/regclient/regclient/pull/652
