// Package main implements the octoctl application.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-orb/go-orb/codecs"
	"github.com/go-orb/go-orb/log"
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

func createConfig(ctx context.Context, cmd *cli.Command) (context.Context, error) {
	logger, err := log.New(log.WithLevel(cmd.String("log-level")))
	if err != nil {
		return ctx, err
	}

	// Set timeout for Downloads
	cfgCtx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	logger.Debug("Creating configuration", "config", cmd.StringSlice("config"))

	cfg, err := octoconfig.New(logger, cmd.StringSlice("config"))
	if err != nil {
		return ctx, err
	}

	if err := cfg.Run(cfgCtx); err != nil {
		return ctx, err
	}

	ctx = context.WithValue(ctx, configKey{}, cfg)

	return ctx, nil
}

func configShow(ctx context.Context, cmd *cli.Command) error {
	cfg := ctx.Value(configKey{}).(*octoconfig.Config) //nolint:errcheck

	codec, ok := codecs.Plugins.Get(cmd.String("format"))
	if !ok {
		return fmt.Errorf("unknown format: %s", cmd.String("format"))
	}

	b, err := codec.Marshal(cfg.Data)
	if err != nil {
		return err
	}

	//nolint:forbidigo
	fmt.Println(string(b))

	return nil
}

func createComposse(ctx context.Context, cmd *cli.Command) error {
	cfg := ctx.Value(configKey{}).(*octoconfig.Config) //nolint:errcheck

	data := cfg.Data

	delete(data, "configs")
	delete(data, "octoctl")
	delete(data, "repos")

	projectID := data["projectID"].(string)
	delete(data, "projectID")
	data["name"] = projectID

	services, ok := data["services"].(map[string]any)
	if !ok {
		return fmt.Errorf("services not found")
	}

	for name := range services {
		delete(services[name].(map[string]any), "octocompose")
	}

	codec, err := codecs.GetMime(codecs.MimeYAML)
	if err != nil {
		return err
	}

	b, err := codec.Marshal(data)
	if err != nil {
		return err
	}

	//nolint:forbidigo
	fmt.Println(string(b))

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
		},
		Commands: []*cli.Command{
			{
				Name:  "check",
				Usage: "Runs validation checks.",
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
					{
						Name: "compose",
						Usage: "Creates a Docker Compose file.",
						Before: createConfig,
						Action: createComposse,
					},
				},
			},
			{
				Name:  "autostart",
				Usage: "Manages service autostart settings.",
				Commands: []*cli.Command{
					{
						Name:  "enable",
						Usage: "Enables autostart for a service.",
					},
					{
						Name:  "disable",
						Usage: "Disables autostart for a service.",
					},
					{
						Name:  "status",
						Usage: "Shows the autostart status of all services.",
					},
				},
			},
			{
				Name:  "upgrade",
				Usage: "Upgrades octoctl to the latest version.",
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Error("Error while running the command", "error", err)
		os.Exit(1)
	}
}
