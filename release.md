# Release v0.5.2

Breaking Changes:

- A few interfaces in the blob package were converted to pointers to structs. ([PR 547][pr-547])

Features:

- Expose the underlying OCI Layout Index to `regctl tag ls --format` ([PR 518][pr-518])
- Support compression on image export and import. ([PR 522][pr-522])
- Image mod of timestamps is now aware of base images. ([PR 524][pr-524])
- Add reproducible method / option to image mod. ([PR 525][pr-525])
- Support setting labels on specific platforms with image mod. ([PR 528][pr-528])
- Add `WithFileTarTime` method and `regctl image mod --file-tar-time` option to edit timestamps inside tar files. ([PR 530][pr-530])
- Support digest-tags in artifact list and tree output ([PR 531][pr-531])
- Add support for decompressing xz layers ([PR 534][pr-534])
- Support getting an artifact from an index of artifacts ([PR 536][pr-536])
- Add repo filters to regsync when copying registries ([PR 538][pr-538])
- Add gosec security linter ([PR 541][pr-541])
- Refactor type/blob package. ([PR 547][pr-547])
- Support pushing artifact to an index entry with `regctl artifact put --index` ([PR 548][pr-548])

Fixes:

- Add size limits on manifests ([PR 512][pr-512])
- Always set `artifactType` with `regctl artifact put` ([PR 513][pr-513])
- Manifest delete should not fail when referrer file is missing ([PR 515][pr-515])
- Artifact put of referrer should not add a manifest reference in ocidir ([PR 515][pr-515])
- Reproducible image creation scripts should prune stale referrers ([PR 515][pr-515])
- Fail faster on image copy when target registry is unreachable ([PR 517][pr-517])
- Avoid changing docker build attestations when converting to referrers ([PR 527][pr-527])

Chores:

- Cleanup docs on regclient package. ([PR 543][pr-543])
- Upgrade yaml package to v3. ([PR 544][pr-544])

New Contributors:

- @fanthos

[pr-512]: https://github.com/regclient/regclient/pull/512
[pr-513]: https://github.com/regclient/regclient/pull/513
[pr-515]: https://github.com/regclient/regclient/pull/515
[pr-517]: https://github.com/regclient/regclient/pull/517
[pr-518]: https://github.com/regclient/regclient/pull/518
[pr-522]: https://github.com/regclient/regclient/pull/522
[pr-524]: https://github.com/regclient/regclient/pull/524
[pr-525]: https://github.com/regclient/regclient/pull/525
[pr-527]: https://github.com/regclient/regclient/pull/527
[pr-528]: https://github.com/regclient/regclient/pull/528
[pr-530]: https://github.com/regclient/regclient/pull/530
[pr-531]: https://github.com/regclient/regclient/pull/531
[pr-534]: https://github.com/regclient/regclient/pull/534
[pr-536]: https://github.com/regclient/regclient/pull/536
[pr-538]: https://github.com/regclient/regclient/pull/538
[pr-541]: https://github.com/regclient/regclient/pull/541
[pr-543]: https://github.com/regclient/regclient/pull/543
[pr-544]: https://github.com/regclient/regclient/pull/544
[pr-547]: https://github.com/regclient/regclient/pull/547
[pr-548]: https://github.com/regclient/regclient/pull/548
