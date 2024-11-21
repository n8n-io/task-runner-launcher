# task-runner-launcher

CLI utility to launch [n8n task runners](https://docs.n8n.io/PENDING).

```sh
./task-runner-launcher launch -type javascript
2024/11/15 13:53:33 Starting to execute `launch` command...
2024/11/15 13:53:33 Loaded config file loaded with a single runner config
2024/11/15 13:53:33 Changed into working directory: /Users/ivov/Development/task-runner-launcher/bin
2024/11/15 13:53:33 Filtered environment variables
2024/11/15 13:53:33 Authenticated with n8n main instance
2024/11/15 13:53:33 Launching runner...
2024/11/15 13:53:33 Command: /usr/local/bin/node
2024/11/15 13:53:33 Args: [/Users/ivov/Development/n8n/packages/@n8n/task-runner/dist/start.js]
2024/11/15 13:53:33 Env vars: [LANG PATH TERM N8N_RUNNERS_N8N_URI N8N_RUNNERS_GRANT_TOKEN]
```

## Setup

### Install

- Install Node.js >=18.17 
- Install n8n >= PENDING_VERSION
- Download launcher binary from [releases page](https://github.com/n8n-io/task-runner-launcher/releases)

### Config

Create a config file for the launcher at `/etc/n8n-task-runners.json`.

Sample config file:

```json
{
  "task-runners": [
    {
      "runner-type": "javascript",
      "workdir": "/usr/local/bin",
      "command": "/usr/local/bin/node",
      "args": [
        "/usr/local/lib/node_modules/n8n/node_modules/@n8n/task-runner/dist/start.js"
      ],
      "allowed-env": [
        "PATH",
        "N8N_RUNNERS_GRANT_TOKEN",
        "N8N_RUNNERS_N8N_URI",
        "N8N_RUNNERS_MAX_PAYLOAD",
        "N8N_RUNNERS_MAX_CONCURRENCY",
        "NODE_FUNCTION_ALLOW_BUILTIN",
        "NODE_FUNCTION_ALLOW_EXTERNAL",
        "NODE_OPTIONS"
      ]
    }
  ]
}
```

Task runner config fields:

- `runner-type`: Type of task runner, currently only `javascript` supported
- `workdir`: Path to directory containing the task runner binary
- `command`: Command to execute to start task runner
- `args`: Args for command to execute, currently path to task runner entrypoint
- `allowed-env`: Env vars allowed to be passed to the task runner

### Auth

Generate a secret auth token (e.g. random string) for the launcher to authenticate with the n8n main instance. You will need to pass that token as `N8N_RUNNERS_AUTH_TOKEN` to the n8n main instance and to the launcher. During the `launch` command, the launcher will exchange this auth token for a short-lived grant token from the n8n instance, and pass the grant token to the runner.

### Idle timeout

If idle for 15 seconds, the runner will exit. To override this duration, set the `N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT` to a number of seconds, or `0` to disable it. After runner exit on idle timeout, the launcher will re-launch the runner on demand, i.e. when the next task comes in.   

## Usage

Once setup is complete, start the launcher:

```sh
export N8N_RUNNERS_AUTH_TOKEN=...
export N8N_RUNNERS_N8N_URI=... 
./task-runner-launcher javascript
```

## Development

1. Install Go >=1.23, `golangci-lint` and `make`

2. Clone repo and create a [config file](#config)

```sh
git clone https://github.com/n8n-io/PENDING-NAME
cd PENDING_NAME
touch config.json && echo '<json-config-content>' > config.json
sudo mv config.json /etc/n8n-task-runners.json
```

3. Make changes to launcher.

4. Start n8n:

```sh
export N8N_RUNNERS_ENABLED=true
export N8N_RUNNERS_MODE=external 
export N8N_RUNNERS_LAUNCHER_PATH=...
export N8N_RUNNERS_AUTH_TOKEN=...
pnpm start
```

5. Build and run launcher:

```sh
export N8N_RUNNERS_N8N_URI=...
export N8N_RUNNERS_AUTH_TOKEN=...

make run
```

## Release

1. Create a git tag following semver:

```sh
git tag 1.2.0
git push origin v1.2.0
```

2. Publish a [GitHub release](https://github.com/n8n-io/task-runner-launcher/releases/new) with the tag. The [`release` workflow](./.github/workflows/release.yml) will build binaries for arm64 and amd64 and upload them to the release in the [releases page](https://github.com/n8n-io/task-runner-launcher/releases).

> [!IMPORTANT]  
> Mark the GitHub release as `latest` and NOT as `pre-release` or the `release` workflow will not run.