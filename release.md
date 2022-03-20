# Release v0.4.1

Breaking Changes:

- Blob methods have been updated to use a descriptor instead of the digest and size.
  ([PR 173][pr-173])

New Features:

- Allow the user-agent to be overridden.
  ([PR 172][pr-172])
- Support the Data field in the descriptor on manifest and blob gets.
  ([PR 174][pr-174])
- Added image modification functionality.
  ([PR 182][pr-182])

Bug Fixes:

- Fix an issue with dangling references in the `ocidir` `index.json`.
  ([PR 176][pr-176])
- Fix handling of relative paths in `ocidir`.
  ([PR 177][pr-177])
- Fix handling of `/etc/docker/certs.d`.
  ([PR 179][pr-179])
- Fix handling of registry CA configuration.
  ([PR 180][pr-180])

[pr-172]: https://github.com/regclient/regclient/pull/172
[pr-173]: https://github.com/regclient/regclient/pull/173
[pr-174]: https://github.com/regclient/regclient/pull/174
[pr-176]: https://github.com/regclient/regclient/pull/176
[pr-177]: https://github.com/regclient/regclient/pull/177
[pr-179]: https://github.com/regclient/regclient/pull/179
[pr-180]: https://github.com/regclient/regclient/pull/180
[pr-182]: https://github.com/regclient/regclient/pull/182
