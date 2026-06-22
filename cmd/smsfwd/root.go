package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tbxark/air780e-sms-forwarder/internal/config"
	"github.com/tbxark/air780e-sms-forwarder/internal/forwarder"
	"github.com/tbxark/air780e-sms-forwarder/internal/serialport"
)

var BuildVersion = "dev"

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
		Use:   "smsfwd",
		Short: "Read Air780E SMS messages and forward them",
	}

	root.AddCommand(newForwardCommand())
	root.AddCommand(newPortsCommand())
	root.AddCommand(newVersionCommand())
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
	cfg := config.Default()
	baud := cfg.Baud
	probe := true
	probeTimeout := serialport.DefaultProbeTimeout
	cmd := &cobra.Command{
		Use:   "ports",
		Short: "List serial port candidates",
		Long:  "List detected serial ports, probe candidates with a bare AT command, and mark the port auto-detect would choose.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if probe {
				if baud <= 0 {
					return fmt.Errorf("invalid baud %d", baud)
				}
				if probeTimeout <= 0 {
					return fmt.Errorf("invalid probe timeout %s", probeTimeout)
				}
				serialport.PrintProbedCandidates(baud, probeTimeout)
			} else {
				serialport.PrintCandidates()
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&baud, "baud", baud, "baud rate used for AT probes")
	cmd.Flags().BoolVar(&probe, "probe", probe, "probe each candidate with a bare AT command")
	cmd.Flags().DurationVar(&probeTimeout, "probe-timeout", probeTimeout, "timeout per AT probe")
	return cmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), BuildVersion)
			return err
		},
	}
}
