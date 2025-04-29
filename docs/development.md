# Development

To set up a development environment, follow these steps:

1. Install Go >=1.24.2, [`golangci-lint`](https://golangci-lint.run/welcome/install/) and `make`.

2. Clone this repository and create a [config file](setup.md).

```sh
git clone https://github.com/n8n-io/task-runner-launcher
cd task-runner-launcher
touch config.json && echo '<json-config-content>' > config.json
sudo mv config.json /etc/n8n-task-runners.json
```

3. Make your changes.

4. Build launcher:

```sh
make build
```

5. Start n8n >= 1.69.0:

```sh
export N8N_RUNNERS_ENABLED=true
export N8N_RUNNERS_MODE=external
export N8N_RUNNERS_AUTH_TOKEN=...     # random string
pnpm start
```

6. Start launcher:

```sh
export N8N_RUNNERS_AUTH_TOKEN=...     # same string as in step 5
make run
```

> [!TIP]
> You can use `N8N_RUNNERS_LAUNCHER_LOG_LEVEL=debug` for granular logging and `NO_COLOR=1` to disable color output.
