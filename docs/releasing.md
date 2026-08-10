# Releasing

Merges to `main` run CI and the reusable release workflow. Conventional Commits are evaluated by `svu`; when a new semantic version is required, automation creates an annotated tag and publishes:

- a GitHub Release and Linux manager archives through GoReleaser
- `linux/amd64` and `linux/arm64` images at `ghcr.io/pierinho13/external-service-discovery-operator`
- a signed OCI chart at `oci://ghcr.io/pierinho13/charts/external-service-discovery-operator`

Before merging, run `make release-check`. Release publishing requires Actions write access to contents/packages/attestations and the Helm signing secrets documented in the repository README.
