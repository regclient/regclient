# Release v0.5.4

New Features:

- Add `regctl --host` flag to configure registries for a single command. ([PR 572][pr-572])
- Configure HTTP client timeouts. ([PR 584][pr-584])
- Add `regclient.Ping` method. ([PR 590][pr-590])
- regctl: warn on failed logins or bad registry configuration changes. ([PR 590][pr-590])

Fixes:

- Fix handling of invalid hostname in `regclient.RepoList`. ([PR 577][pr-577])
- Fix bug in regsync tag filtering when running as a server. ([PR 579][pr-579])
- Enable parallel builds of the make "binaries" target. ([PR 588][pr-588])

Chores:

- Update Go docs on blob APIs and the config. ([PR 573][pr-573])
- Refactor the Ref package. ([PR 587][pr-587])

Contributors:

- @andyli
- @Juneezee
- @sudo-bmitch

[pr-572]: https://github.com/regclient/regclient/pull/572
[pr-573]: https://github.com/regclient/regclient/pull/573
[pr-577]: https://github.com/regclient/regclient/pull/577
[pr-579]: https://github.com/regclient/regclient/pull/579
[pr-584]: https://github.com/regclient/regclient/pull/584
[pr-587]: https://github.com/regclient/regclient/pull/587
[pr-588]: https://github.com/regclient/regclient/pull/588
[pr-590]: https://github.com/regclient/regclient/pull/590
