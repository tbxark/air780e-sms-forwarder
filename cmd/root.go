package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tbxark/air780e-sms-forwarder/internal/app"
)

func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := NewRootCommand()
	root.SetContext(ctx)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func NewRootCommand() *cobra.Command {
	cfg := app.DefaultConfig()

	root := &cobra.Command{
		Use:   "air780e-sms-forwarder",
		Short: "Read Air780E SMS messages and forward them",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.ListPorts {
				app.PrintSerialCandidates()
				return nil
			}
			return app.Run(cmd.Context(), cfg)
		},
	}

	root.Flags().StringVar(&cfg.Port, "port", cfg.Port, "serial port path")
	root.Flags().IntVar(&cfg.Baud, "baud", cfg.Baud, "serial baud rate")
	root.Flags().BoolVar(&cfg.ListPorts, "list-ports", false, "list serial port candidates and exit")
	root.Flags().BoolVar(&cfg.ConfigurePort, "configure", cfg.ConfigurePort, "deprecated compatibility flag; serial settings are configured by the serial library")
	root.Flags().BoolVar(&cfg.InitModem, "init", cfg.InitModem, "send AT commands for SMS text-mode push")
	root.Flags().BoolVar(&cfg.TelegramRaw, "telegram-raw", cfg.TelegramRaw, "also forward raw serial lines to Telegram")
	root.Flags().StringVar(&cfg.TelegramToken, "telegram-token", cfg.TelegramToken, "Telegram bot token")
	root.Flags().StringVar(&cfg.TelegramChat, "telegram-chat", cfg.TelegramChat, "Telegram chat id")

	root.AddCommand(newPortsCommand())
	return root
}

func newPortsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "ports",
		Short: "List serial port candidates",
		RunE: func(cmd *cobra.Command, args []string) error {
			app.PrintSerialCandidates()
			return nil
		},
	}
}
