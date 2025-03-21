package octoconfig

import (
	"net/url"
	"testing"

	"github.com/go-orb/go-orb/config"
	"github.com/stretchr/testify/require"
)

func TestRepoFileUnmarshal(t *testing.T) {
	u, err := url.Parse("repo_test.yaml")
	require.NoError(t, err)

	// Read the test YAML file
	yamlData, err := config.Read(u)
	require.NoError(t, err)

	// Unmarshal the YAML into our repo struct
	var repo RepoFile
	err = config.Parse(nil, "repos", yamlData, &repo)
	require.NoError(t, err)

	// Verify the structure was parsed correctly
	// Includes
	require.NotNil(t, repo.Include)
	require.Len(t, repo.Include, 1)
	require.Equal(t, "./service/webdav.yaml", repo.Include[0].URL.String())
	require.Equal(t, "./service/webdav.yaml.asc", repo.Include[0].GPG.String())

	// Operator
	require.NotNil(t, repo.Operators)
	require.Contains(t, repo.Operators, "baremetal")

	baremetalOperator := repo.Operators["baremetal"]

	// Binary
	require.NotNil(t, baremetalOperator.Binary, "Binary config should not be nil")
	require.NotNil(t, baremetalOperator.Binary["linux_amd64"], "LinuxAMD64 config should not be nil")
	require.Equal(t, "https://github.com/octocompose/operator-baremetal/releases/download/v0.0.1/operator-baremetal-linux-amd64",
		baremetalOperator.Binary["linux_amd64"].URL.String())
	require.Equal(t, "operator-baremetal",
		baremetalOperator.Binary["linux_amd64"].Binary)

	// Source
	require.NotNil(t, baremetalOperator.Source, "Source config should not be nil")
	require.Equal(t, "https://github.com/octocompose/operator-baremetal.git",
		baremetalOperator.Source.URL.String())
	require.Equal(t, "refs/tags/v0.0.1",
		baremetalOperator.Source.Ref)
	require.Len(t, baremetalOperator.Source.BuildCmds, 1)
	require.Equal(t, "dist/{{OS}}/{{ARCH}}/operator-baremetal",
		baremetalOperator.Source.Binary)

	// Tool
	require.NotNil(t, repo.Tools)
	require.Contains(t, repo.Tools, "check-tcp")

	checkTcpTool := repo.Tools["check-tcp"]

	// Docker config
	require.NotNil(t, checkTcpTool.Docker, "Docker config should not be nil")
	require.Equal(t, "docker.io", checkTcpTool.Docker.Registry)
	require.Equal(t, "octocompose/tools", checkTcpTool.Docker.Image)
	require.Equal(t, "v0.0.1", checkTcpTool.Docker.Tag)
	require.Equal(t, "/usr/local/bin/check-tcp", checkTcpTool.Docker.Entrypoint)
}
