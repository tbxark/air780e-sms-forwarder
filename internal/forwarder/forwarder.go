package forwarder

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/tbxark/air780e-sms-forwarder/internal/config"
	"github.com/tbxark/air780e-sms-forwarder/internal/modem"
	"github.com/tbxark/air780e-sms-forwarder/internal/notifier"
	"github.com/tbxark/air780e-sms-forwarder/internal/serialport"
	"github.com/tbxark/air780e-sms-forwarder/internal/sms"
)

func Run(ctx context.Context, cfg config.Config) error {
	if cfg.Port == "" {
		port, err := serialport.AutoDetect()
		if err != nil {
			return fmt.Errorf("serial port not found: %w\nset port in config.json", err)
		}
		cfg.Port = port
	}

	fmt.Printf("Using serial port: %s @ %d\n", cfg.Port, cfg.Baud)
	if !cfg.ConfigurePort {
		fmt.Println("Serial configuration flag is deprecated; the serial library configures the port when opening")
	}
	port, err := serialport.Open(cfg.Port, cfg.Baud)
	if err != nil {
		return fmt.Errorf("open serial failed: %w", err)
	}
	defer port.Close()

	notifiers := notifier.Build(cfg)
	if len(notifiers) == 0 {
		fmt.Println("Message forwarding disabled")
	} else {
		for _, n := range notifiers {
			fmt.Printf("%s forwarding enabled\n", n.Name())
		}
	}

	events := make(chan sms.Event, 8)
	rawLines := make(chan string, 32)
	atModem := modem.NewAT(port, rawLines, events)

	if cfg.InitModem {
		if err := modem.InitAir780E(atModem); err != nil {
			return fmt.Errorf("initialize modem failed: %w", err)
		}
	}

	fmt.Println("Listening. Send an SMS to the SIM card now. Press Ctrl+C to stop.")
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Stopped")
			return nil
		case <-atModem.Closed():
			fmt.Println("Serial connection closed")
			return nil
		case line := <-rawLines:
			fmt.Printf("%s RAW %s\n", time.Now().Format(time.RFC3339), line)
			if cfg.TelegramRaw {
				for _, n := range notifiers {
					if err := n.SendRaw(ctx, line); err != nil {
						fmt.Fprintf(os.Stderr, "%s raw send failed: %v\n", n.Name(), err)
					}
				}
			}
		case event := <-events:
			fmt.Printf("%s SMS from=%s text=%s\n", event.At.Format(time.RFC3339), event.From, event.Text)
			for _, n := range notifiers {
				if err := n.SendSMS(ctx, event); err != nil {
					fmt.Fprintf(os.Stderr, "%s sms send failed: %v\n", n.Name(), err)
				}
			}
		}
	}
}
