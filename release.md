# Release v0.4.3

Breaking Changes:

- Media type variable for docker image layers has been renamed ([PR 220][pr-220])

New Features:

- Improve handling of external URLs ([PR 220][pr-220])
- Call credential helpers only when needed ([PR 234][pr-234])
- Support credential helpers directly in regsync and regbot ([PR 238][pr-238])
- Allow pushing artifacts and manifest by digest ([PR 242][pr-242])

Bug Fixes:

- regctl image mod args for expose and volume rm actually rm ([PR 216][pr-216])
- manifest head request now set the descriptor size correctly ([PR 222][pr-222])
- Fix for chunked uploads ([PR 228][pr-228])
- Fix image import from a multi-platform image to an ocidir:// layout ([PR 235][pr-235])

Other Changes:

- Adding experimental support for OCI referrers ([PR 225][pr-225])
- Adds experimental referrer support to regctl artifact commands ([PR 226][pr-226])
- Experimental: adding support for OCI artifact media type ([PR 229][pr-229])
- Switch to internal processing of docker config.json ([PR 234][pr-234])

[pr-216]: https://github.com/regclient/regclient/pull/216
[pr-220]: https://github.com/regclient/regclient/pull/220
[pr-222]: https://github.com/regclient/regclient/pull/222
[pr-225]: https://github.com/regclient/regclient/pull/225
[pr-226]: https://github.com/regclient/regclient/pull/226
[pr-228]: https://github.com/regclient/regclient/pull/228
[pr-229]: https://github.com/regclient/regclient/pull/229
[pr-234]: https://github.com/regclient/regclient/pull/234
[pr-235]: https://github.com/regclient/regclient/pull/235
[pr-238]: https://github.com/regclient/regclient/pull/238
[pr-242]: https://github.com/regclient/regclient/pull/242
