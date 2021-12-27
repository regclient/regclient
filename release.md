# Release v0.3.10

Bug fixes:

- Insecure TLS should handle unknown TLS keys
- regsync backup errors are now a warning only, allowing automated recovery from a corrupt registry
- Fix for deleting tags pointing to an OCI manifest

Features and Changes:

- Verifying media type and digest on manifests
- Adding option to delete manifests by tag instead of digest
- Image copy: support added for digest tags used by tools like projectsigstore/cosign
- Image copy: support added to force a recursive copy when manifest already exists at target
- Handle registries without HEAD support (older versions of Nexus)
