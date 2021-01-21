# Release v0.2.1

- `regctl registry` commands `login`, `logout`, and `set` previously required
  the nonintuitive DNS name "registry-1.docker.io" for Hub. They now accept
  "docker.io" and default to Hub when no registry is provided.
