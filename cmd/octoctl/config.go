package main

var hardcodedConfig = `
repos:
  operators:
    docker:
      binary:
        linux_amd64:
          url: https://github.com/octocompose/operator-docker/releases/download/v0.0.6/operator-docker_0.0.6_linux_amd64
          sha256Url: https://github.com/octocompose/operator-docker/releases/download/v0.0.6/operator-docker_0.0.6_linux_amd64.sha256
        linux_arm64:
          url: https://github.com/octocompose/operator-docker/releases/download/v0.0.6/operator-docker_0.0.6_linux_arm64
          sha256Url: https://github.com/octocompose/operator-docker/releases/download/v0.0.6/operator-docker_0.0.6_linux_arm64.sha256
      source:
        # If path is set and existing, repo and ref are ignored.
        path: ../operator-docker
        repo: https://github.com/octocompose/operator-docker.git
        ref: refs/heads/main
        buildCmds:
          - GOOS={{.OS}} GOARCH={{.ARCH}} make build
        binary: dist/{{.OS}}/{{.ARCH}}/operator-docker
`
