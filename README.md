# octoctl

## Overview

`octoctl` is the command-line tool for [OctoCompose](https://octocompose.dev/), at its core it is a wrapper for `docker compose`, `podman-compose` and `nerdctl compose`.

It extends the functionality of those by adding includes of url's, templates and inline config. See the ready-to-install [penpot](https://github.com/octocompose/charts/blob/main/examples/penpot.yaml) and [OpenCloud](https://github.com/octocompose/charts/blob/main/examples/opencloud.yaml) apps.

Instead of cloning `github` repos configuring an `.env` file you download a `config.yaml` configure it for you needs and run the app.

## Installing

### From Release

```sh
curl -sL https://get.octocompose.dev | sh
```

### From Source

```sh
go install github.com/octocompose/octoctl/cmd/octoctl@main
```

### Pre-build Apps

- [OpenCloud](https://github.com/octocompose/charts/blob/main/examples/opencloud.yaml)
- [OpenCloud with external ingress](https://github.com/octocompose/charts/blob/main/examples/opencloud-exterrnal-ingress.yaml)
- [Penpot](https://github.com/octocompose/charts/blob/main/examples/penpot.yaml)

## Usage

```sh
octoctl --help
```

### The `octoctl compose` command

This command is special as it needs `--` to separate the flags for `octoctl` from the flags for `docker compose`.

```sh
octoctl -c config.yaml compose -- --help
```

## Development

### Prerequisites

- [Go 1.24.1](https://golang.org/dl/)

### Build and Run

```sh
make && ./dist/linux/amd64/octoctl start -c config.yaml --force-build-operator -l debug
```

## Authors

- [jochumdev](https://github.com/jochumdev), [blog](https://jochum.dev/)

## License

[Apache License 2.0](https://github.com/octocompose/octoctl/blob/main/LICENSE)
