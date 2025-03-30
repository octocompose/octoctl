# octoctl

> In development, nothing works yet :)

## What works

```
go run ./cmd/octoctl/... config show -c config.yaml
```

## Overview

`octoctl` is the command-line tool for [OctoCompose](https://octocompose.dev/).

## Installation

### From Source

```sh
go install github.com/octocompose/octoctl/cmd/octoctl@main
```

## Usage

```sh
octoctl --help
```

## Development

### Prerequisites

- [Go 1.24.1](https://golang.org/dl/)

### Build and Run

```sh
make && ./dist/linux/amd64/octoctl start -c config.yaml -c development.yaml --force-build-operator -l debug
```

## Authors

- [jochumdev](https://github.com/jochumdev), [blog](https://jochum.dev/)

## License

[Apache License 2.0](https://github.com/octocompose/octoctl/blob/main/LICENSE)
