# Release v0.5.5

New Features:

- Add OpenSSF Best Practices Badge. ([PR 607][pr-607])
- Adding OpenSSF Scorecard badge and GHA workflow. ([PR 609][pr-609])

Fixes:

- Validate references in regclient methods. ([PR 595][pr-595])
- Data race in the reghttp fallback timeout handling. ([PR 599][pr-599])
- HTTP proxy using environment variables. ([PR 615][pr-615])

Chores:

- Reorder descriptor fields. ([PR 594][pr-594])
- Add test for ocidir throttle race. ([PR 601][pr-601])
- Add gomajor utility to Makefile. ([PR 602][pr-602])
- Add commands to Makefile for managing releases. ([PR 604][pr-604])
- Pin GitHub actions. ([PR 605][pr-605])
- Use full semver on dependencies where available. ([PR 605][pr-605])
- Adjust token permissions on GitHub actions. ([PR 606][pr-606])
- Include disclosure timeline in security policy. ([PR 608][pr-608])
- Improve contributor guidelines. ([PR 612][pr-612])
- Improve BlobPut tests. ([PR 613][pr-613])

Contributors:

- @peusebiu
- @sudo-bmitch

[pr-594]: https://github.com/regclient/regclient/pull/594
[pr-595]: https://github.com/regclient/regclient/pull/595
[pr-599]: https://github.com/regclient/regclient/pull/599
[pr-601]: https://github.com/regclient/regclient/pull/601
[pr-602]: https://github.com/regclient/regclient/pull/602
[pr-604]: https://github.com/regclient/regclient/pull/604
[pr-605]: https://github.com/regclient/regclient/pull/605
[pr-606]: https://github.com/regclient/regclient/pull/606
[pr-607]: https://github.com/regclient/regclient/pull/607
[pr-608]: https://github.com/regclient/regclient/pull/608
[pr-609]: https://github.com/regclient/regclient/pull/609
[pr-612]: https://github.com/regclient/regclient/pull/612
[pr-613]: https://github.com/regclient/regclient/pull/613
[pr-615]: https://github.com/regclient/regclient/pull/615
