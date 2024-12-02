# Release

1. Create a git tag following semver:

```sh
git tag 1.2.0
git push origin v1.2.0
```

2. Publish a [GitHub release](https://github.com/n8n-io/task-runner-launcher/releases/new) with the tag. 

The [`release` workflow](../.github/workflows/release.yml) will build binaries for arm64 and amd64 and upload them to the release in the [releases page](https://github.com/n8n-io/task-runner-launcher/releases).

> [!WARNING]
> Mark the GitHub release as `latest` and NOT as `pre-release` or the `release` workflow will not run.

3. Update the `LAUNCHER_VERSION` argument in two Dockerfiles of the main repository:

- `docker/images/n8n/Dockerfile`
- `docker/images/n8n-custom/Dockerfile`
