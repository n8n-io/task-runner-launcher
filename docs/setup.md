# Setup

To set up the launcher:

1. Download the latest launcher binary from the [releases page](https://github.com/n8n-io/task-runner-launcher/releases).

2. Create a [config file](#config-file) on the host and make it accessible to the launcher.

3. Configure [environment variables](#environment-variables).

4. Deploy the launcher as a sidecar container to an n8n main or worker instance, setting the launcher to manage one or multiple runner types.

```sh
./task-runner-launcher javascript # or
./task-runner-launcher javascript python
```

5. Ensure your orchestrator (e.g. k8s) performs regular liveness checks on both launcher and task broker.

- The launcher exposes a health check endpoint at `/healthz` on port `5680`, configurable via `N8N_RUNNERS_LAUNCHER_HEALTH_CHECK_PORT`.
- The task broker exposes a health check endpoint at `/healthz` on port `5679`, configurable via `N8N_RUNNERS_BROKER_PORT`.

<br>

```mermaid
sequenceDiagram
    participant k8s
    participant N8N as n8n main or worker
    participant Launcher
    participant Runner
    Note over N8N: main server: 5678<br/>broker server: 5679
    Note over Launcher: health check server: 5680
    Note over Runner: health check server: 5681
    
    par k8s health checks
        k8s->>Launcher: GET /healthz
    and
        k8s->>N8N: GET /healthz
    end
    
    Launcher->>Runner: GET /healthz
    N8N<<->>Runner: ws heartbeat
```

## Config file

Example config file at `/etc/n8n-task-runners.json`:

```json
{
  "task-runners": [
    {
      "runner-type": "javascript",
      "workdir": "/usr/local/bin",
      "command": "/usr/local/bin/node",
      "args": [
        "--disallow-code-generation-from-strings",
        "--disable-proto=delete",
        "/usr/local/lib/node_modules/n8n/node_modules/@n8n/task-runner/dist/start.js"
      ],
      "health-check-server-port": "5681",
      "allowed-env": ["PATH", "GENERIC_TIMEZONE"],
      "env-overrides": {
        "N8N_RUNNERS_TASK_TIMEOUT": "80",
        "N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT": "120",
        "N8N_RUNNERS_MAX_CONCURRENCY": "3",
        "NODE_FUNCTION_ALLOW_BUILTIN": "crypto",
        "NODE_FUNCTION_ALLOW_EXTERNAL": "moment",
        "NODE_OPTIONS": "--max-old-space-size=4096"
      }
    },
    {
      "runner-type": "python",
      "workdir": "/usr/local/bin",
      "command": "/usr/local/bin/python",
      "args": [
        "/usr/local/lib/python3.13/site-packages/n8n/task-runner-python/main.py"
      ],
      "health-check-server-port": "5682", 
      "allowed-env": ["PATH", "GENERIC_TIMEZONE"],
      "env-overrides": {
        "N8N_RUNNERS_TASK_TIMEOUT": "30",
        "N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT": "30",
        "N8N_RUNNERS_MAX_CONCURRENCY": "2"
      }
    }
  ]
}
```

| Property       | Description                                                                                                             |
| --------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `runner-type`   | Type of task runner, e.g. `javascript` or `python`. The launcher can manage only one runner per type.                   |
| `workdir`       | Path where the task runner's `command` will run.                                                                                          |
| `command`       | Command to start the task runner.                                                                                       |
| `args`          | Args and flags to use with `command`.                                                                                           |
| `health-check-server-port` | Port for the runner's health check server. When a single runner is configured, this is optional and defaults to `5681`. When multiple runners are configured, this is required and must be unique per runner.
| `allowed-env`   | Env vars that the launcher will pass through from its own environment to the runner. See [environment](environment.md). |
| `env-overrides` | Env vars that the launcher will set directly on the runner. See [environment](environment.md).                          |

## Environment variables

It is required to pass `N8N_RUNNERS_AUTH_TOKEN` to the launcher and to the n8n instance. This token will allow the launcher to authenticate with the n8n instance and to obtain a grant tokens for every runner it manages. All other env vars are optional and are listed in the [n8n docs](https://docs.n8n.io/hosting/configuration/environment-variables/task-runners).

The launcher can pass env vars to task runners in two ways, as specified in the [config file](#config-file):

| Source | Description | Purpose |
|--------|-------------|------------|
| `allowed-env` | Env vars filtered from the launcher's own environment | Passing env vars common to all runner types |
| `env-overrides` | Env vars set by the launcher directly on the runner, with precedence over `allowed-env` | Passing env vars specific to a single runner type |

Exceptionally, these four env vars cannot be disallowed or overridden:

- `N8N_RUNNERS_TASK_BROKER_URI`
- `N8N_RUNNERS_GRANT_TOKEN`
- `N8N_RUNNERS_HEALTH_CHECK_SERVER_ENABLED=true`
- `N8N_RUNNERS_HEALTH_CHECK_SERVER_PORT`