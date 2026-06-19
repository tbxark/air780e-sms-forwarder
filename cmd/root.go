package cmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tbxark/air780e-sms-forwarder/internal/config"
	"github.com/tbxark/air780e-sms-forwarder/internal/forwarder"
	"github.com/tbxark/air780e-sms-forwarder/internal/serialport"
)

func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := NewRootCommand()
	root.SetContext(ctx)
	if err := root.Execute(); err != nil {
		slog.Error("command failed", "err", err)
		os.Exit(1)
	}
}

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "air780e-sms-forwarder",
		Short: "Read Air780E SMS messages and forward them",
	}

	root.AddCommand(newForwardCommand())
	root.AddCommand(newPortsCommand())
	return root
}

func newForwardCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "forward [config]",
		Short: "Listen for SMS messages and forward them",
		Long:  "Listen to the Air780E serial port, parse SMS modem indications, and forward messages through configured notification channels. If config is omitted, config.json is used.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath := config.DefaultPath
			if len(args) > 0 {
				configPath = args[0]
			}

			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			return forwarder.Run(cmd.Context(), cfg)
		},
	}
}

func newPortsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "ports",
		Short: "List serial port candidates",
		Long:  "List detected serial ports, with Air780E and stable device paths ranked first when available.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			serialport.PrintCandidates()
			return nil
		},
	}
}
