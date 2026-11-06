# Releases

This document describes the release cadence and published artifacts for the
 Kubernetes MCP Server.

## Release cadence

Releases are scheduled every four weeks. Maintainers coordinate each release
and confirm it is ready to publish.

- Release sooner when important bug fixes or security fixes warrant it.
- Delay a scheduled release when testing identifies a release blocker.

Merging a change does not guarantee a particular release date. Users can follow
[GitHub releases](https://github.com/containers/kubernetes-mcp-server/releases)
for published versions and release notes.

## Versions and support

Release tags use the format `vMAJOR.MINOR.PATCH`, for example `v0.0.70`.
While the project is in the `0.x` series, releases may include new features and
behavior changes alongside fixes. Review the release notes before upgrading;
configuration migrations belong in [Configuration Changes](configuration-changes.md).

Fixes are delivered in new releases from `main`. The project does not maintain
long-term support or backport branches. See the [security policy](../SECURITY.md)
for security support and private vulnerability reporting.

## Published artifacts

| Artifact | Publishing workflow |
|----------|---------------------|
| GitHub release with native binaries, plus npm and Python packages | [Release](../.github/workflows/release.yaml) |
| Version-tagged container images on Quay and GHCR | [Container images](../.github/workflows/release-image.yml) |
| Versioned Helm chart | [Helm chart](../.github/workflows/release-helm.yaml) |
| MCP Registry entry | [MCP Registry](../.github/workflows/release-mcp-registry.yaml) |
| Platform-specific MCP bundles attached to the GitHub release | [MCP bundles](../.github/workflows/release-mcpb.yaml) |

The `latest` container tag follows `main`; use a version tag when deploying a
release.
