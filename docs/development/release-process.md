# Release Process

This repository includes serveral Go modules: The "main" Go module `sigs.k8s.io/multicluster-runtime` and several provider modules.

The release process is partially automated. The following steps need to be taken:

1. Create a git tag via `git tag -am 'v0.20.4' v0.20.4` (see [Versioning](#versioning)).
2. Push the tag to the repository via `git push origin v0.20.4`.
3. Check if GitHub Actions [workflow](../../.github/workflows/release.yaml) created commit and provider-specific git tags on `main` or `release-` branch.

## Versioning

`sigs.k8s.io/multicluster-runtime` follows the `sigs.k8s.io/controller-runtime` version for ease of use (e.g. the version of multicluster-runtime that depends on controller-runtime `v0.20.4` must also be `v0.20.4`). Make sure that [go.mod](../../go.mod) contains the right dependency before tagging a new release.
