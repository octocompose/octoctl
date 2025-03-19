# octoctl

> In development, nothing works yet :)

## What works

```
go run ./cmd/octoctl/... config show -c config.yaml
```

### Docker compose

```
go run ./cmd/octoctl/... config show -c config.yaml > compose.yaml
```

Run `docker compose up` and fix errors, then run it again, until it starts the services.

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

## Authors

- [jochumdev](https://github.com/jochumdev), [blog](https://jochum.dev/)

## License

[Apache License 2.0](https://github.com/octocompose/octoctl/blob/main/LICENSE)
