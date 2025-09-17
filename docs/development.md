# Development

To set up a development environment, follow these steps:

1. Install [Bazelisk](https://github.com/bazelbuild/bazelisk#installation). Bazelisk manages Bazel versions
   automatically.

```bash
# macOS
brew install bazelisk

# Linux/Windows - see https://github.com/bazelbuild/bazelisk#installation
```

Go and other development tools are managed automatically by Bazel.

2. Clone this repository and create a [config file](setup.md#config-file).

```sh
git clone https://github.com/n8n-io/task-runner-launcher
cd task-runner-launcher
touch config.json && echo '<json-config-content>' > config.json
sudo mv config.json /etc/n8n-task-runners.json
```

3. Make your changes.

4. Build launcher:

```sh
bazel build //cmd/launcher:task-runner-launcher
```

5. Format and lint code (hermetic - no local Go required):

```sh
bazel run //:fmt    # Format code
bazel run //:lint   # Basic linting
```

6. Run tests with coverage:

```sh
bazel test //...            # Run all tests
bazel run //:coverage       # Generate coverage report (91.3% currently)
```

7. Start n8n >= 1.69.0:

```sh
export N8N_RUNNERS_ENABLED=true
export N8N_RUNNERS_MODE=external
export N8N_RUNNERS_AUTH_TOKEN=test
pnpm start
```

8. Start launcher:

```sh
export N8N_RUNNERS_AUTH_TOKEN=test
bazel-bin/cmd/launcher/task-runner-launcher_/task-runner-launcher javascript
# Or run directly:
bazel run //cmd/launcher:task-runner-launcher -- javascript
```

## Development Commands

| Task              | Bazel Command                                                 | Description               |
|-------------------|---------------------------------------------------------------|---------------------------|
| **Build**         | `bazel build //cmd/launcher:task-runner-launcher`             | Build main binary         |
| **Test**          | `bazel test //...`                                            | Run all tests             |
| **Coverage**      | `bazel run //:coverage`                                       | Generate coverage report  |
| **Format**        | `bazel run //:fmt`                                            | Format Go code (hermetic) |
| **Lint**          | `bazel run //:lint`                                           | Basic linting (hermetic)  |
| **Cross-compile** | `bazel build //cmd/launcher:task-runner-launcher-linux-amd64` | Build for Linux AMD64     |
| **Cross-compile** | `bazel build //cmd/launcher:task-runner-launcher-linux-arm64` | Build for Linux ARM64     |

## Benefits of Bazel Build System

- **Zero Setup**: No need to install Go, golangci-lint, or other tools locally
- **Hermetic Builds**: Reproducible builds across all environments
- **Fast Incremental**: Intelligent caching makes rebuilds ultra-fast
- **Cross-compilation**: Built-in support for Linux AMD64/ARM64
- **Coverage**: Professional LCOV reports with 91.3% current coverage
- **Version Management**: Bazelisk handles Bazel versions automatically

> [!TIP]
> You can use `N8N_RUNNERS_LAUNCHER_LOG_LEVEL=debug` for granular logging and `NO_COLOR=1` to disable color output.
