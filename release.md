# Release v0.5.0

The two key features are:

- Updating the image copy to copy layers concurrently and with an improved UI.
- Update support for OCI with the Referrers changes coming in their 1.1 releases.

Image Copy:

- Add progress display to `regctl image copy` ([PR 413][pr-413])
- Image copy is now run with concurrency. ([PR 419][pr-419])
- Fix `regctl image copy` output on narrow terminals. ([PR 440][pr-440])
- Add a fast check option for copying images with referrers and digest tags. ([PR 441][pr-441])
- Update `regctl image copy` for tty displays. ([PR 447][pr-447])

OCI Support:

- Add support for `artifactType` in image manifest ([PR 400][pr-400])
- Accept manifests with OCI artifact media type (experimental). ([PR 418][pr-418])
- Handle the OCI-Subject header to detect referrer support. ([PR 446][pr-446])
- Embed the `Platform` field directly in the `ImageConfig` ([PR 456][pr-456])
- Switch from scratch to empty JSON media type ([PR 463][pr-463])
- Support artifactType and subject fields on OCI Index ([PR 476][pr-476])

Other Features:

- Image mod pushes directly to the target ref without an extra copy step ([PR 438][pr-438])
- Performance improvements for regsync ([PR 449][pr-449])
- Support client certs and keys for mTLS registry auth. ([PR 454][pr-454])
- Support updating annotations on platform specific manifests in a manifest list. ([PR 457][pr-457])
- Add ability to sort referrers by annotation. ([PR 467][pr-467])
- Use `SOURCE_DATE_EPOCH` build arg support in buildkit. ([PR 472][pr-472])
- Add regctl tag list filtering ([PR 477][pr-477])
- Add option to import a specific image or tag from an export of multiple images ([PR 482][pr-482])

Fixes:

- Invalid references are detected before querying the registry ([PR 414][pr-414])
- Fix handling of content-type headers. ([PR 418][pr-418])
- Fix race when creating ocidir ([PR 420][pr-420])
- Avoid an internal race condition when managing the referrers fallback tag. ([PR 427][pr-427])
- Fix: close reader when converting a blob to an OCI config ([PR 434][pr-434])
- Support manifests missing a mediaType field. ([PR 436][pr-436])
- Fix GitHub badges. ([PR 437][pr-437])
- Handle symlinks in the tar file with `regctl image import` ([PR 452][pr-452])
- Fix GCR credential helper to work on Artifact Registry ([PR 455][pr-455])
- Fix deadlock when referrers or digest tags refer to a parent manifest ([PR 464][pr-464])
- Improve error handling of `regctl artifact tree`. ([PR 470][pr-470])
- Fix copy when both referrers and digest-tags are included. ([PR 471][pr-471])
- Fix handling of copy with looping referrers or digest-tags to validating registries ([PR 475][pr-475])

[pr-400]: https://github.com/regclient/regclient/pull/400
[pr-413]: https://github.com/regclient/regclient/pull/413
[pr-414]: https://github.com/regclient/regclient/pull/414
[pr-418]: https://github.com/regclient/regclient/pull/418
[pr-419]: https://github.com/regclient/regclient/pull/419
[pr-420]: https://github.com/regclient/regclient/pull/420
[pr-427]: https://github.com/regclient/regclient/pull/427
[pr-434]: https://github.com/regclient/regclient/pull/434
[pr-436]: https://github.com/regclient/regclient/pull/436
[pr-437]: https://github.com/regclient/regclient/pull/437
[pr-438]: https://github.com/regclient/regclient/pull/438
[pr-440]: https://github.com/regclient/regclient/pull/440
[pr-441]: https://github.com/regclient/regclient/pull/441
[pr-446]: https://github.com/regclient/regclient/pull/446
[pr-447]: https://github.com/regclient/regclient/pull/447
[pr-449]: https://github.com/regclient/regclient/pull/449
[pr-452]: https://github.com/regclient/regclient/pull/452
[pr-454]: https://github.com/regclient/regclient/pull/454
[pr-455]: https://github.com/regclient/regclient/pull/455
[pr-456]: https://github.com/regclient/regclient/pull/456
[pr-457]: https://github.com/regclient/regclient/pull/457
[pr-463]: https://github.com/regclient/regclient/pull/463
[pr-464]: https://github.com/regclient/regclient/pull/464
[pr-467]: https://github.com/regclient/regclient/pull/467
[pr-470]: https://github.com/regclient/regclient/pull/470
[pr-471]: https://github.com/regclient/regclient/pull/471
[pr-472]: https://github.com/regclient/regclient/pull/472
[pr-475]: https://github.com/regclient/regclient/pull/475
[pr-476]: https://github.com/regclient/regclient/pull/476
[pr-477]: https://github.com/regclient/regclient/pull/477
[pr-482]: https://github.com/regclient/regclient/pull/482
