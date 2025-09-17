# Release

1. Publish a [GitHub release](https://github.com/n8n-io/task-runner-launcher/releases/new) with a git tag following semver.

The [`release` workflow](../.github/workflows/release.yml) will build binaries for arm64 and amd64 using Bazel and
upload them to the release in the [releases page](https://github.com/n8n-io/task-runner-launcher/releases).

> [!WARNING]
> When publishing the GitHub release, mark it as `latest` and NOT as `pre-release` or the `release` workflow will not run.

2. Update the `LAUNCHER_VERSION` argument in `docker/images/n8n/Dockerfile` and `docker/images/runners/Dockerfile` in the main repository.

## Build System

The project uses **Bazel** for reproducible, hermetic builds. The release workflow uses Bazel to:

- Build cross-platform binaries (Linux AMD64/ARM64)
- Generate deterministic builds across environments
- Manage all dependencies automatically
- Ensure consistent build artifacts
