# Environment

The launcher passes two kinds of environment variables to task runners:

- Env vars from the launcher's own environment, if allowed by `allowed-env` in `n8n-task-runners.json`.
- Env vars set by the launcher directly on the runner, as specified in `env-overrides` in `n8n-task-runners.json`. This is useful for setting different env vars on separate runner types. On conflict, env vars in `env-overrides` take precedence over env vars in `allowed-env`.

Example:

```json
{
  "task-runners": [
    {
      "allowed-env": [
        "PATH",
        "GENERIC_TIMEZONE",
        "N8N_RUNNERS_MAX_PAYLOAD",
        "N8N_RUNNERS_MAX_CONCURRENCY",
        "N8N_RUNNERS_TASK_TIMEOUT",
      ],
      "override-envs": [
        "NODE_FUNCTION_ALLOW_BUILTIN=crypto",
        "NODE_FUNCTION_ALLOW_EXTERNAL=moment"
        "NODE_OPTIONS=--max-old-space-size=4096"
      ]
    },
  ]
}
```

Exceptionally, these three env vars cannot be disallowed or overriden:

- `N8N_RUNNERS_TASK_BROKER_URI`
- `N8N_RUNNERS_GRANT_TOKEN`
- `N8N_RUNNERS_HEALTH_CHECK_SERVER_ENABLED=true`
