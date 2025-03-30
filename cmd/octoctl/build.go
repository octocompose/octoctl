package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-orb/go-orb/config"
	"github.com/go-orb/go-orb/log"
	"github.com/google/shlex"
	"github.com/octocompose/octoctl/pkg/octocache"
	"github.com/octocompose/octoctl/pkg/octoconfig"
)

func cloneRepo(logger log.Logger, cfg *octoconfig.Config, url *config.URL, referenceName string, forcePull bool) (string, error) {
	sha256sum := sha256.Sum256([]byte(url.String()))

	cachePath, err := octocache.Path(cfg.ProjectID, "build", hex.EncodeToString(sha256sum[:16]))
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(cachePath); err != nil {
		logger.Debug("Cloning git repository", "repository", url.String(), "cachePath", cachePath)

		if _, err := git.PlainClone(cachePath, false, &git.CloneOptions{
			URL:               url.String(),
			Depth:             1,
			RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
			Progress:          os.Stderr,
			SingleBranch:      true,
			ReferenceName:     plumbing.ReferenceName(referenceName),
		}); err != nil {
			return "", err
		}
	}

	if forcePull { //nolint:nestif
		logger.Debug("Pulling git repository", "repository", url.String(), "cachePath", cachePath)

		r, err := git.PlainOpen(cachePath)
		if err != nil {
			return "", err
		}

		// Get the working directory for the repository
		w, err := r.Worktree()
		if err != nil {
			return "", err
		}

		// Pull the latest changes from the origin remote and merge into the current branch
		err = w.Pull(&git.PullOptions{
			Depth:             1,
			RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
			Progress:          os.Stderr,
			SingleBranch:      true,
			RemoteName:        "origin",
			ReferenceName:     plumbing.ReferenceName(referenceName),
		})
		if err != nil {
			if errors.Is(err, git.NoErrAlreadyUpToDate) {
				return cachePath, nil
			}

			return "", err
		}
	}

	return cachePath, nil
}

// renderBinaryName processes template variables in the binary name.
func renderBinaryName(logger log.Logger, buildInfo *octoconfig.RepoSource, templateVars map[string]any) error {
	t, err := template.New("build").Parse(buildInfo.Binary)
	if err != nil {
		logger.Error("Error while parsing binary template", "error", err)
		return fmt.Errorf("while parsing binary template: %w", err)
	}

	buf := &bytes.Buffer{}
	if err := t.Execute(buf, templateVars); err != nil {
		logger.Error("Error while executing binary template", "error", err)
		return fmt.Errorf("while executing binary template: %w", err)
	}

	buildInfo.Binary = buf.String()

	return nil
}

// runBuildCommands executes the build commands in the specified directory.
func runBuildCommands(
	ctx context.Context,
	logger log.Logger,
	buildInfo *octoconfig.RepoSource,
	dir string,
	templateVars map[string]any,
) error {
	// Split the BuildCmds strings and execute each command
	for _, cmdStr := range buildInfo.BuildCmds {
		// Process template
		t, err := template.New("build").Parse(cmdStr)
		if err != nil {
			logger.Error("Error while parsing build command", "error", err)
			return fmt.Errorf("while parsing build command: %w", err)
		}

		buf := &bytes.Buffer{}
		if err := t.Execute(buf, templateVars); err != nil {
			logger.Error("Error while executing build command template", "error", err)
			return fmt.Errorf("while parsing build command '%s': %w", buf.String(), err)
		}

		logger.Debug("Running build command", "command", buf.String(), "dir", dir)

		// Parse command and environment variables
		parsedCmd, err := shlex.Split(buf.String())
		if err != nil {
			logger.Error("Error while parsing build command", "error", err)
			return fmt.Errorf("while parsing build command '%s': %w", buf.String(), err)
		}

		cmd := ""
		env := []string{}
		args := []string{}

		for idx, arg := range parsedCmd {
			if !strings.Contains(arg, "=") {
				cmd = arg
				args = parsedCmd[idx+1:]

				break
			}

			env = append(env, arg)
		}

		// Execute command
		execCmd := exec.CommandContext(ctx, cmd, args...) //nolint:gosec

		execCmd.Env = append(os.Environ(), env...)

		execCmd.Dir = dir

		if logger.Level() >= log.LevelDebug {
			execCmd.Stdout = os.Stdout
			execCmd.Stderr = os.Stderr
		}

		if err := execCmd.Run(); err != nil {
			logger.Error("Error while running build command", "error", err)
			return fmt.Errorf("while running build command: %w", err)
		}
	}

	return nil
}

// build builds a binary from source or returns a cached binary.
func build(
	ctx context.Context,
	logger log.Logger,
	cfg *octoconfig.Config,
	buildInfo *octoconfig.RepoSource,
	forceBuild bool,
) (string, error) {
	// Process template variables for binary name
	templateVars := cfg.TemplateVars()
	if err := renderBinaryName(logger, buildInfo, templateVars); err != nil {
		return "", err
	}

	// Try to find existing binary
	if buildInfo.Path != nil && !forceBuild {
		path := filepath.Join(buildInfo.Path.Path, buildInfo.Binary)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Determine build directory and binary path
	var dir, binaryPath string

	var err error

	// Clone repository if needed
	if buildInfo.Path == nil {
		dir, err = cloneRepo(logger, cfg, buildInfo.Repo, buildInfo.Ref, forceBuild)
		if err != nil {
			logger.Error("Error while cloning repository", "repository", buildInfo.Repo.String(), "error", err)
			return "", err
		}

		binaryPath = filepath.Join(dir, buildInfo.Binary)
	} else {
		dir = buildInfo.Path.Path
		binaryPath = filepath.Join(dir, buildInfo.Binary)
	}

	// Return existing binary if available and not forcing rebuild
	if !forceBuild {
		if _, err := os.Stat(binaryPath); err == nil {
			return binaryPath, nil
		}
	}

	// Run build commands
	if err := runBuildCommands(ctx, logger, buildInfo, dir, templateVars); err != nil {
		return "", err
	}

	// Verify binary was created
	if _, err := os.Stat(binaryPath); err != nil {
		logger.Error("Error build didn't produce a binary", "binary", binaryPath, "error", err)
		return "", fmt.Errorf("build didn't produce binary %s: %w", binaryPath, err)
	}

	return binaryPath, nil
}
