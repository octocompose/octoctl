#!/bin/bash

# Detect OS and Arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

# Normalize Arch names
case $ARCH in
  x86_64) ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
esac

# Get the latest version of octoctl
LATEST_VERSION=$(curl -s https://api.github.com/repos/octocompose/octoctl/releases/latest | grep -oP '"tag_name": "\K[^"]+')

# Download and install octoctl
mkdir -p ~/.local/bin
curl -L "https://github.com/octocompose/octoctl/releases/download/${LATEST_VERSION}/octoctl_${LATEST_VERSION#v}_${OS}_${ARCH}" -o ~/.local/bin/octoctl
chmod +x ~/.local/bin/octoctl

echo "octoctl ${LATEST_VERSION} installed successfully to ~/.local/bin/octoctl"