// Package main implements the octoctl application.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/go-orb/go-orb/codecs"
	"github.com/go-orb/go-orb/log"
	"github.com/octocompose/octoctl/pkg/octocache"
	"github.com/octocompose/octoctl/pkg/octoconfig"

	"github.com/urfave/cli/v3"

	"github.com/earthboundkid/versioninfo/v2"

	_ "github.com/go-orb/plugins/codecs/json"
	_ "github.com/go-orb/plugins/codecs/toml"
	_ "github.com/go-orb/plugins/codecs/yaml"
	_ "github.com/go-orb/plugins/config/source/file"
	_ "github.com/go-orb/plugins/log/slog"
)

// Version is the version of the octoctl application.
//
//nolint:gochecknoglobals
var Version = versioninfo.Short()

type configKey struct{}
type loggerKey struct{}

func createConfig(ctx context.Context, cmd *cli.Command) (context.Context, error) {
	logger, err := log.New(log.WithLevel(cmd.String("log-level")))
	if err != nil {
		return ctx, err
	}

	// Set timeout for Downloads
	cfgCtx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	logger.Debug("Creating configuration", "config", cmd.StringSlice("config"))

	codec, err := codecs.GetMime(codecs.MimeYAML)
	if err != nil {
		logger.Error("Error while getting codec", "error", err)
		return ctx, err
	}

	hardCodedData := map[string]any{}
	if err := codec.Unmarshal([]byte(hardcodedConfig), &hardCodedData); err != nil {
		logger.Error("Error while marshaling configuration", "error", err)
		return ctx, err
	}

	cfg, err := octoconfig.New(logger, cmd.StringSlice("config"), hardCodedData)
	if err != nil {
		logger.Error("Error while creating configuration", "error", err)
		return ctx, err
	}

	if err := cfg.Run(cfgCtx); err != nil {
		logger.Error("Error while running configuration", "error", err)
		return ctx, err
	}

	ctx = context.WithValue(ctx, configKey{}, cfg)
	ctx = context.WithValue(ctx, loggerKey{}, logger)

	return ctx, nil
}

func configShow(ctx context.Context, cmd *cli.Command) error {
	cfg := ctx.Value(configKey{}).(*octoconfig.Config) //nolint:errcheck
	logger := ctx.Value(loggerKey{}).(log.Logger)      //nolint:errcheck

	codec, ok := codecs.Plugins.Get(cmd.String("format"))
	if !ok {
		logger.Error("Unknown format", "format", cmd.String("format"))
		return fmt.Errorf("unknown format: %s", cmd.String("format"))
	}

	b, err := codec.Marshal(cfg.Data)
	if err != nil {
		logger.Error("Error while marshaling configuration", "error", err)
		return fmt.Errorf("while marshaling configuration: %w", err)
	}

	//nolint:forbidigo
	fmt.Println(string(b))

	return nil
}

func runOperator(ctx context.Context, cmd *cli.Command, args []string) error {
	cfg := ctx.Value(configKey{}).(*octoconfig.Config) //nolint:errcheck
	logger := ctx.Value(loggerKey{}).(log.Logger)      //nolint:errcheck

	forceBuild := cmd.Bool("force-build-operator")

	if cfg.Octoctl.Operator == "" {
		return errors.New("operator not specified")
	}

	operatorRepo, ok := cfg.Repo.Operators[cfg.Octoctl.Operator]
	if !ok {
		logger.Error("Operator not found", "operator", cfg.Octoctl.Operator)
		return fmt.Errorf("operator '%s' not found", cfg.Octoctl.Operator)
	}

	osArch := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)

	binary, ok := operatorRepo.Binary[osArch]
	if !ok && operatorRepo.Source == nil {
		logger.Error("Operator not available for architecture", "operator", cfg.Octoctl.Operator, "osArch", osArch)
		return fmt.Errorf("operator '%s' not available for %s", cfg.Octoctl.Operator, osArch)
	}

	var execPath string

	if ok && !forceBuild {
		url, err := octocache.CachedURL(ctx, cfg.ProjectID, binary.URL, binary.SHA256URL, "operators", false)
		if err != nil {
			logger.Error("Error while getting cached URL", "error", err)
			return fmt.Errorf("while getting cached URL: %w", err)
		}

		if err := os.Chmod(url.Path, 0o700); err != nil {
			logger.Error("Error while chmoding cached operator", "error", err)
			return fmt.Errorf("while chmoding cached operator: %w", err)
		}

		logger.Debug("Using cached operator", "url", binary.URL, "cached", url.Path)
		execPath = url.Path
	}

	if execPath == "" && operatorRepo.Source != nil {

		path, err := build(ctx, logger, cfg, operatorRepo.Source, forceBuild)
		if err != nil {
			return err
		}

		logger.Debug("Using git operator", "url", operatorRepo.Source.Repo, "path", path)
		execPath = path
	}

	if execPath == "" {
		logger.Error("Operator not available for architecture", "operator", cfg.Octoctl.Operator, "osArch", osArch)
		return fmt.Errorf("operator '%s' not available for %s", cfg.Octoctl.Operator, osArch)
	}

	codec, err := codecs.GetMime(codecs.MimeJSON)
	if err != nil {
		return err
	}

	b, err := codec.Marshal(cfg.Data)
	if err != nil {
		return err
	}

	logger.Debug("Running operator", "path", execPath, "args", args)

	execCmd := exec.CommandContext(ctx, execPath, args...) //nolint:gosec
	execCmd.Stdin = bytes.NewReader(b)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	if err := execCmd.Run(); err != nil {
		os.Exit(execCmd.ProcessState.ExitCode())
	}

	return nil
}

func main() {
	cmd := &cli.Command{
		Name:    "octoctl",
		Version: Version,
		Usage:   "Service Orchestration Made Simple",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-level",
				Aliases: []string{"l"},
				Value:   "info",
				Usage:   "Set the log level (debug, info, warn, error)",
			},
			&cli.StringSliceFlag{
				Name:     "config",
				Aliases:  []string{"c"},
				Usage:    "Path to configuration files",
				Required: true,
			},
			&cli.BoolFlag{
				Name:  "force-build-operator",
				Usage: "Force build the operator.",
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "Starts the services.",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name: "dry-run",
					},
				},
				Before: createConfig,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := []string{"--log-level", cmd.String("log-level"), "start"}
					if cmd.Bool("dry-run") {
						args = append(args, "--dry-run")
					}
					return runOperator(ctx, cmd, args)
				},
			},
			{
				Name:  "stop",
				Usage: "Stops the services.",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name: "dry-run",
					},
				},
				Before: createConfig,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := []string{"--log-level", cmd.String("log-level"), "stop"}
					if cmd.Bool("dry-run") {
						args = append(args, "--dry-run")
					}
					return runOperator(ctx, cmd, args)
				},
			},
			// {
			// 	Name:  "restart",
			// 	Usage: "Restarts the services.",
			// 	Flags: []cli.Flag{
			// 		&cli.BoolFlag{
			// 			Name: "dry-run",
			// 		},
			// 	},
			// 	Before: createConfig,
			// 	Action: func(ctx context.Context, cmd *cli.Command) error {
			// 		args := []string{"--log-level", cmd.String("log-level"), "restart"}
			// 		if cmd.Bool("dry-run") {
			// 			args = append(args, "--dry-run")
			// 		}
			// 		return runOperator(ctx, cmd, args)
			// 	},
			// },
			{
				Name:  "logs",
				Usage: "Shows logs from services.",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "follow",
						Aliases: []string{"f"},
						Usage:   "Follow the logs.",
					},
				},
				Before: createConfig,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := []string{"--log-level", cmd.String("log-level"), "logs"}
					if cmd.Bool("follow") {
						args = append(args, "--follow")
					}
					return runOperator(ctx, cmd, args)
				},
			},
			{
				Name:   "status",
				Usage:  "Shows status of services.",
				Before: createConfig,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := []string{"--log-level", cmd.String("log-level"), "status"}
					return runOperator(ctx, cmd, args)
				},
			},
			{
				Name:   "show",
				Usage:  "Shows the running configuration.",
				Before: createConfig,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := []string{"--log-level", cmd.String("log-level"), "show"}
					return runOperator(ctx, cmd, args)
				},
			},
			{
				Name:  "config",
				Usage: "Manages the service configurations.",
				Commands: []*cli.Command{
					{
						Name:  "show",
						Usage: "Shows the merged configuration.",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "format",
								Aliases: []string{"f"},
								Value:   "yaml",
								Usage:   "Output format (json, yaml, toml)",
							},
						},
						Before: createConfig,
						Action: configShow,
					},
					{
						Name:  "diff",
						Usage: "Shows differences between configurations.",
					},
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		os.Exit(1)
	}
}
