// Package main implements the octoctl application.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/octocompose/octoctl/pkg/codecs"
	"github.com/octocompose/octoctl/pkg/config"
	"github.com/urfave/cli/v3"

	"github.com/earthboundkid/versioninfo/v2"
)

// Version is the version of the octoctl application.
//
//nolint:gochecknoglobals
var Version = versioninfo.Short()

func configShow(ctx context.Context, cmd *cli.Command) error {
	// Set timeout for Downloads
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	cfg, err := NewConfig(cmd.StringSlice("config"))
	if err != nil {
		return err
	}

	if err := cfg.Run(ctx); err != nil {
		return err
	}

	codec, err := codecs.GetFormat(cmd.String("format"))
	if err != nil {
		return err
	}

	b, err := config.Dump(codec.Mime, cfg.Data)
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
			&cli.StringSliceFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to configuration files",
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))

			return ctx, nil
		},
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "Starts all services.",
			},
			{
				Name:  "stop",
				Usage: "Stops all services.",
			},
			{
				Name:  "restart",
				Usage: "Restarts all services.",
			},
			{
				Name:  "status",
				Usage: "Shows the status of all services.",
			},
			{
				Name:  "logs",
				Usage: "Shows the logs of all services.",
			},
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
						Action: configShow,
					},
					{
						Name:  "diff",
						Usage: "Shows differences between configurations.",
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
		slog.Error("Error while running the command", "error", err)
		os.Exit(1)
	}
}
