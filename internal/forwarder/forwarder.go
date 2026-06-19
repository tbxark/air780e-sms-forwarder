package forwarder

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tbxark/air780e-sms-forwarder/internal/config"
	"github.com/tbxark/air780e-sms-forwarder/internal/modem"
	"github.com/tbxark/air780e-sms-forwarder/internal/serialport"
	"github.com/tbxark/air780e-sms-forwarder/internal/sms"
	"github.com/tbxark/air780e-sms-forwarder/internal/telegrambot"
)

func Run(ctx context.Context, cfg config.Config) error {
	if cfg.TelegramToken == "" {
		return fmt.Errorf("telegram_token is required")
	}
	if _, err := telegrambot.ParseChatID(cfg.TelegramChat); err != nil {
		return err
	}

	if cfg.Port == "" {
		port, err := serialport.AutoDetect()
		if err != nil {
			return fmt.Errorf("serial port not found: %w\nset port in config.json", err)
		}
		cfg.Port = port
	}

	slog.Info("using serial port", "port", cfg.Port, "baud", cfg.Baud)
	if !cfg.ConfigurePort {
		slog.Warn("serial configuration flag is deprecated; the serial library configures the port when opening")
	}
	port, err := serialport.Open(cfg.Port, cfg.Baud)
	if err != nil {
		return fmt.Errorf("open serial failed: %w", err)
	}
	defer port.Close()

	events := make(chan sms.Event, 8)
	rawLines := make(chan string, 32)
	atModem := modem.NewAT(port, rawLines, events)

	if cfg.InitModem {
		if err := modem.InitAir780E(atModem); err != nil {
			return fmt.Errorf("initialize modem failed: %w", err)
		}
	}

	executor := telegrambot.NewMutexExecutor(func(_ context.Context, cmd string) ([]string, error) {
		return modem.RunATCommand(atModem, cmd)
	})
	telegram, err := telegrambot.New(cfg, executor)
	if err != nil {
		return err
	}
	if err := telegram.Initialize(ctx); err != nil {
		_ = telegram.Close(context.Background())
		return fmt.Errorf("initialize telegram bot failed: %w", err)
	}
	pollCtx, cancelPoll := context.WithCancel(ctx)
	defer func() {
		cancelPoll()
		_ = telegram.Close(context.Background())
	}()
	go func() {
		if err := telegram.Start(pollCtx); err != nil && pollCtx.Err() == nil {
			slog.Error("telegram polling failed", "err", err)
		}
	}()
	slog.Info("telegram forwarding and polling control enabled")

	slog.Info("listening for SMS; press Ctrl+C to stop")
	for {
		select {
		case <-ctx.Done():
			slog.Info("stopped")
			return nil
		case <-atModem.Closed():
			slog.Info("serial connection closed")
			return nil
		case line := <-rawLines:
			slog.Info("raw line received", "at", time.Now().Format(time.RFC3339), "line", line)
			if cfg.TelegramRaw {
				if err := telegram.SendRaw(ctx, line); err != nil {
					slog.Error("telegram raw send failed", "err", err)
				}
			}
		case event := <-events:
			slog.Info("sms received", "at", event.At.Format(time.RFC3339), "from", event.From, "text", event.Text)
			if err := telegram.SendSMS(ctx, event); err != nil {
				slog.Error("telegram sms send failed", "err", err)
			}
		}
	}
}
