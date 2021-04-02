# Release v0.3.0

Deprecated:

- Multiple regclient APIs have received breaking changes.
- Configuration file handling has been removed from regclient. Configuration
  should be injected by options when creating the regclient instance.
- DNS and Schema configuration options have been removed. Schema is now handled
  using the TLS setting, and DNS has been separated into Hostname and Mirrors.

Command Changes:

- Docker Hub is now handled as `docker.io` in various commands and is configured
  as the default when no registry is provided.
- Redesign of the mirror handling allows each mirror to have different
  credentials and a path prefix option.
- `raw`, `raw-headers`, and `raw-body` formats added for viewing the original
  response from the registry server.
- `version` sub-commands added to output the tag and git commit.
- `printPretty` format added to output tags and manifest list in a more user
  friendly format.
- regbot creates a new reference instead of reusing an existing one.
- regbot reference adds a digest get/set operation.
- Documentation has been updated.
- `regctl image digest` on a manifest list only performs one GET request instead
  of two.
- Docker schema v1 is supported to handle older images.

regclient API Changes:

- User-Agent in headers now reports which regclient command and commit.
- Retryable code now maintains state across multiple requests to avoid retrying
  a down mirror for each layer of an image.
- API handling to the registry has been abstracted to hopefully allow registry
  specific API requests.
- Error handling updated to support `errors.Is()`.
- Image config moved behind the blob interface.
