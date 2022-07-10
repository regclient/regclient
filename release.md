# Release v0.4.4

Breaking Changes:

- Redundant manifest methods have been deprecated ([PR 257][pr-257])
- Manifest methods specific to Images and Indexes/ManifestLists have a different interface ([PR 258][pr-258])

New Features:

- `regctl blob put` supports a format string ([PR 255][pr-255])
- `regsync` immediately syncs any tags missing on the target without waiting for first sync schedule ([PR 256][pr-256])
- Adding `regctl index` command to create and mutate an Index or Manifest List ([PR 260][pr-260])

Bug Fixes:

- Credential helper support for Docker Hub ([PR 246][pr-246])
- Fix handling of `DOCKER_CONFIG` variable ([PR 249][pr-249])
- Fix handling of custom TLS settings on a registry with authentication ([PR 253][pr-253])

Other Changes:

- Normalize parsing of registry names in various components ([PR 247][pr-247])
- Exclude tags from referrers when copying digest tags ([PR 250][pr-250])

[pr-246]: https://github.com/regclient/regclient/pull/246
[pr-247]: https://github.com/regclient/regclient/pull/247
[pr-249]: https://github.com/regclient/regclient/pull/249
[pr-250]: https://github.com/regclient/regclient/pull/250
[pr-253]: https://github.com/regclient/regclient/pull/253
[pr-255]: https://github.com/regclient/regclient/pull/255
[pr-256]: https://github.com/regclient/regclient/pull/256
[pr-257]: https://github.com/regclient/regclient/pull/257
[pr-258]: https://github.com/regclient/regclient/pull/258
[pr-260]: https://github.com/regclient/regclient/pull/260
