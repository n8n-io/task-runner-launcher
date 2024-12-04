# Setup

This launcher is intended for deployment as a sidecar container alongside one or more n8n instance containers. The launcher exposes a health check endpoint at `/healthz` on port `5680` for liveness checks, and the n8n instance does so on port `5681`. The orchestrator (e.g. k8s) can use this to monitor the health of the launcher and of the n8n instance.

1. Download the **launcher binary** from the [releases page](https://github.com/n8n-io/task-runner-launcher/releases).

2. Create a **config file** on the host at `/etc/n8n-task-runners.json` and make this file accessible to the launcher.

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
        "GENERIC_TIMEZONE",
        "N8N_RUNNERS_GRANT_TOKEN",
        "N8N_RUNNERS_TASK_BROKER_URI",
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

- `runner-type` is the type of task runner, currently only `javascript` is supported.
- `workdir` is the path to directory containing the task runner binary.
- `command` is the command to execute in order to start the task runner.
- `args` are the args for the command to execute, currently the path to the task runner entrypoint.
- `allowed-env` are the env vars that the launcher is allowed to pass to the task runner.

3. Set the **environment variables** for the launcher.

- It is required to specify an auth token by setting `N8N_RUNNERS_AUTH_TOKEN` to a secret. The launcher will use this secret to authenticate with the n8n instance. You will also pass this `N8N_RUNNERS_AUTH_TOKEN` to the n8n instance as well.

- Optionally, specify the launcher's log level by setting `N8N_RUNNERS_LAUNCHER_LOG_LEVEL` to `debug`, `info`, `warn` or `error`. Default is `info`. You can also use `NO_COLOR=1` to disable color output.

- Optionally, specify the launcher's auto-shutdown timeout by setting `N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT` to a number of seconds, or set it to `0` to disable. Default is `15`. The runner will exit after this timeout if it is idle for the specified duration, and will be re-launched on demand when the next task comes in.

- Optionally, specify the task broker's URI (i.e. n8n instance's URI) by setting `N8N_RUNNERS_TASK_BROKER_URI`. Default is `http://127.0.0.1:5679`.

- Optionally, specify the port for the launcher's health check server by setting `N8N_RUNNERS_LAUNCHER_HEALTH_CHECK_PORT`. Default is `5680`. When overriding this port, be mindful of port conflicts - by default, the n8n instance uses `5678` for its regular server and `5679` for its task broker server, and the runner uses `5681` for its healthcheck server.

- Optionally, configure [Sentry error tracking](https://docs.sentry.io/platforms/go/configuration/options/) with these env vars:

  - `SENTRY_DSN`
  - `DEPLOYMENT_NAME`: Mapped to `ServerName`
  - `ENVIRONMENT`: Mapped to `Environment`
  - `N8N_VERSION`: Mapped to `Release`

4. Run the launcher:

```sh
./task-runner-launcher javascript
```
