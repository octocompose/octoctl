# octoctl

## Overview

`octoctl` is the command-line tool for [OctoCompose](https://octocompose.dev/).

## Installation

### From Release

```sh
mkdir -p ~/.local/bin
curl -L https://github.com/octocompose/octoctl/releases/download/v0.0.7/octoctl_0.0.7_linux_amd64 -o ~/.local/bin/octoctl
chmod +x ~/.local/bin/octoctl
```

### From Source

```sh
go install github.com/octocompose/octoctl/cmd/octoctl@main
```

### Example Apps

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
