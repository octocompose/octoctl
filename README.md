# octoctl

## Overview

`octoctl` is the command-line tool for [OctoCompose](https://octocompose.dev/).

## Installation

### From Release

```sh
mkdir -p ~/.local/bin
curl -L https://github.com/octocompose/octoctl/releases/download/v0.0.4/octoctl_0.0.4_linux_amd64 -o ~/.local/bin/octoctl
chmod +x ~/.local/bin/octoctl
```

### From Source

```sh
go install github.com/octocompose/octoctl/cmd/octoctl@main
```

## Install `opencloud` with OctoCompose

#### Create a config

```yaml
name: opencloud01

include:
  - url: https://raw.githubusercontent.com/octocompose/charts/refs/heads/main/opencloud-monolith/config/opencloud.yaml
  - url: https://raw.githubusercontent.com/octocompose/charts/refs/heads/main/opencloud-monolith/config/collabora.yaml
  - url: https://raw.githubusercontent.com/octocompose/charts/refs/heads/main/opencloud-monolith/config/tika.yaml
  - url: https://raw.githubusercontent.com/octocompose/charts/refs/heads/main/opencloud-monolith/config/traefik.yaml

  - url: https://raw.githubusercontent.com/octocompose/charts/refs/heads/main/opencloud-monolith/config/web_extensions/all.yaml

configs:
    collabora:
        admin:
            password: notSecure
            user: admin
    opencloud:
        domain:
            collabora: collabora.example.com
            companion: companion.example.com
            oc: cloud.example.com
            onlyoffice: onlyoffice.opencloud.test
            wopiserver: wopiserver.example.com
        idp:
            adminPassword: notSecure
        smtp:
            authentication: plain
            host: mail.example.com
            insecure: "false"
            password: "notSecure"
            port: 587
            sender: OpenCloud notifications <cloud@example.com>
            username: cloud@example.com
octoctl:
  operator: docker
```

#### Run it

```sh
octoctl -c config.yaml start
```

#### Show the `compose.yaml` it generates

```sh
octoctl -c config.yaml show
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
make && ./dist/linux/amd64/octoctl start -c config.yaml --force-build-operator -l debug
```

## Authors

- [jochumdev](https://github.com/jochumdev), [blog](https://jochum.dev/)

## License

[Apache License 2.0](https://github.com/octocompose/octoctl/blob/main/LICENSE)
