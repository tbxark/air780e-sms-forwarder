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

const (
	serialWatchdogCommand    = ""
	serialWatchdogInterval   = time.Minute
	serialWatchdogThreshold  = 3
	serialWatchdogAlertLimit = 10 * time.Second
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
	defer func() {
		if err := port.Close(); err != nil {
			slog.Warn("serial port close failed", "err", err)
		}
	}()

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

	watchdogCtx, cancelWatchdog := context.WithCancel(ctx)
	defer cancelWatchdog()
	go runSerialWatchdog(watchdogCtx, executor, telegram)

	slog.Info("listening for SMS; press Ctrl+C to stop")
	for {
		select {
		case <-ctx.Done():
			slog.Info("stopped")
			return nil
		case <-atModem.Closed():
			slog.Info("serial connection closed")
			sendWatchdogAlert(ctx, telegram, fmt.Sprintf("serial connection closed on %s", cfg.Port))
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

func runSerialWatchdog(ctx context.Context, executor telegrambot.Executor, telegram *telegrambot.Service) {
	ticker := time.NewTicker(serialWatchdogInterval)
	defer ticker.Stop()

	failures := 0
	alerted := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := probeSerial(ctx, executor)
			if err == nil {
				if failures > 0 {
					slog.Info("serial watchdog recovered", "failures", failures)
				}
				failures = 0
				alerted = false
				continue
			}

			failures++
			slog.Warn("serial watchdog probe failed", "failures", failures, "threshold", serialWatchdogThreshold, "err", err)
			if failures < serialWatchdogThreshold || alerted {
				continue
			}

			alerted = true
			reason := fmt.Sprintf("serial watchdog probe failed %d consecutive times: %v", failures, err)
			sendWatchdogAlert(ctx, telegram, reason)
		}
	}
}

func probeSerial(ctx context.Context, executor telegrambot.Executor) error {
	_, err := executor.ExecuteAT(ctx, serialWatchdogCommand)
	return err
}

func sendWatchdogAlert(ctx context.Context, telegram *telegrambot.Service, reason string) {
	alertCtx, cancel := context.WithTimeout(ctx, serialWatchdogAlertLimit)
	defer cancel()
	if err := telegram.SendWatchdogAlert(alertCtx, reason); err != nil {
		slog.Error("telegram watchdog alert send failed", "err", err)
	}
}
