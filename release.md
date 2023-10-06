# Release v0.5.3

Fixes:

- Fix formatting variables in `regctl image inspect`. ([PR 554][pr-554])

New Features:

- Add a `GetSize` method to image manifests (OCI and Docker2 manifests). ([PR 565][pr-565])

Chores:

- Refactoring CLIs to remove global state. ([PR 550][pr-550])
- Set GOTOOLCHAIN=local in CI ([PR 556][pr-556])
- Reorder Go imports to move local packages last. ([PR 557][pr-557])
- Remove duplicated tests from ci-registry action. ([PR 559][pr-559])
- Run tests using t.Parallel where possible. ([PR 564][pr-564])
- Update install guidance for quarantined binaries on MacOS. ([PR 569][pr-569])
- Release notes now include contributors. ([PR 570][pr-570])

Contributors:

- @felipecrs
- @sorenisanerd
- @sudo-bmitch

[pr-550]: https://github.com/regclient/regclient/pull/550
[pr-554]: https://github.com/regclient/regclient/pull/554
[pr-556]: https://github.com/regclient/regclient/pull/556
[pr-557]: https://github.com/regclient/regclient/pull/557
[pr-559]: https://github.com/regclient/regclient/pull/559
[pr-564]: https://github.com/regclient/regclient/pull/564
[pr-565]: https://github.com/regclient/regclient/pull/565
[pr-569]: https://github.com/regclient/regclient/pull/569
[pr-570]: https://github.com/regclient/regclient/pull/570
