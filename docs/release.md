# Release

1. Publish a [GitHub release](https://github.com/n8n-io/task-runner-launcher/releases/new) with a new git tag following semver. The [`release` workflow](../.github/workflows/release.yml) will build binaries for arm64 and amd64 and upload them to the release in the [releases page](https://github.com/n8n-io/task-runner-launcher/releases).

2. Update the `LAUNCHER_VERSION` argument in the main repository:

- `docker/images/runners/Dockerfile`
- `docker/images/runners/Dockerfile.distroless`
