# Release v0.3.8

Breaking changes:

- `regclient.ImageImport` API uses an `io.ReadSeeker` instead of a filename
- OCI export/import is corrected to use annotations, previous exports of an OCI image will not import with this change

Features and Changes:

- Disable default blob max to support registries that don't handle chunked uploads
- Update to Go 1.16 and dependencies have been bumped
- Images now push to GHCR in addition to Docker Hub
- GHA fixes caching for faster docker image builds
- Allow a 201 http status for chunked blob uploads
