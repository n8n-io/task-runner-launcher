# task-runner-launcher

CLI utility to launch [n8n task runners](https://docs.n8n.io/PENDING).

```
./task-runner-launcher javascript
2024/11/21 09:51:08 INFO  Starting to execute `launch` command
2024/11/21 09:51:08 DEBUG Loaded config file loaded with a single runner config
2024/11/21 09:51:08 DEBUG Changed into working directory: /Users/ivov/Development/task-runner-launcher/bin
2024/11/21 09:51:08 DEBUG Filtered environment variables
2024/11/21 09:51:08 DEBUG Fetched grant token for launcher
2024/11/21 09:51:08 INFO  Launcher's runner ID: 1a261be237a38f6d
2024/11/21 09:51:08 DEBUG Connected: ws://127.0.0.1:5679/runners/_ws?id=1a261be237a38f6d
2024/11/21 09:51:08 DEBUG <- Received message `broker:inforequest`
2024/11/21 09:51:08 DEBUG -> Sent message `runner:info`
2024/11/21 09:51:08 DEBUG <- Received message `broker:runnerregistered`
2024/11/21 09:51:08 DEBUG -> Sent message `runner:taskoffer` for offer ID `d67a6d5855d4876d`
2024/11/21 09:51:08 INFO  Waiting for launcher's task offer to be accepted...
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

### Launcher logs

To control the launcher's log level, set the `N8N_LAUNCHER_LOG_LEVEL` env var to `debug`, `info`, `warn` or `error`. Default is `info`.

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
export N8N_RUNNERS_LAUNCHER_PATH=...  # i.e. path/to/launcher/binary
export N8N_RUNNERS_AUTH_TOKEN=...     # i.e. random string
pnpm start
```

5. Build and run launcher:

```sh
export N8N_LAUNCHER_LOG_LEVEL=debug
export N8N_RUNNERS_AUTH_TOKEN=...     # i.e. same string as in step 4

make run
```

## Health check

The launcher exposes a health check endpoint at `/healthz` on port `5681` for liveness checks. 

To override the launcher health check port, set the `N8N_LAUNCHER_HEALTCHECK_PORT` env var. When overriding the default health check port, be mindful of port conflicts - the n8n main instance uses `5678` by default for its HTTP server and `5679` for its task runner server, and the runner uses `5680` by default for its healthcheck server.

## Release

1. Create a git tag following semver:

```sh
git tag 1.2.0
git push origin v1.2.0
```

2. Publish a [GitHub release](https://github.com/n8n-io/task-runner-launcher/releases/new) with the tag. The [`release` workflow](./.github/workflows/release.yml) will build binaries for arm64 and amd64 and upload them to the release in the [releases page](https://github.com/n8n-io/task-runner-launcher/releases).

> [!IMPORTANT]  
> Mark the GitHub release as `latest` and NOT as `pre-release` or the `release` workflow will not run.