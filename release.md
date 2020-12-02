# Release v0.1.0

- Adding regsync command and image to copy images between registries.
- Locking added to regclient to handle concurrent requests.
- Improved formatting / templating functionality.
- Handling images with external layers (copying Windows images).
- Fixing permissions of `/home/appuser` in the `regclient/regctl` and
  `regclient/regsync` images.
- Bug fix for passing multiple host configs to regclient.
