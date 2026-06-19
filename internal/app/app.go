package app

import (
	"context"
	"fmt"
	"os"
	"time"

	modemat "github.com/warthog618/modem/at"
	"go.bug.st/serial"
)

type SMSEvent struct {
	From string
	Text string
	At   time.Time
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Port == "" {
		port, err := autoDetectPort()
		if err != nil {
			return fmt.Errorf("serial port not found: %w\nset AIR780E_PORT or pass -port", err)
		}
		cfg.Port = port
	}

	fmt.Printf("Using serial port: %s @ %d\n", cfg.Port, cfg.Baud)
	if !cfg.ConfigurePort {
		fmt.Println("Serial configuration flag is deprecated; the serial library configures the port when opening")
	}
	port, err := openSerialPort(cfg.Port, cfg.Baud)
	if err != nil {
		return fmt.Errorf("open serial failed: %w", err)
	}
	defer port.Close()

	notifiers := buildNotifiers(cfg)
	if len(notifiers) == 0 {
		fmt.Println("Message forwarding disabled")
	} else {
		for _, n := range notifiers {
			fmt.Printf("%s forwarding enabled\n", n.Name())
		}
	}

	events := make(chan SMSEvent, 8)
	rawLines := make(chan string, 32)
	modem := newATModem(port, rawLines, events)

	if cfg.InitModem {
		if err := initAir780E(modem); err != nil {
			return fmt.Errorf("initialize modem failed: %w", err)
		}
	}

	fmt.Println("Listening. Send an SMS to the SIM card now. Press Ctrl+C to stop.")
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Stopped")
			return nil
		case <-modem.Closed():
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
		case sms := <-events:
			fmt.Printf("%s SMS from=%s text=%s\n", sms.At.Format(time.RFC3339), sms.From, sms.Text)
			for _, n := range notifiers {
				if err := n.SendSMS(ctx, sms); err != nil {
					fmt.Fprintf(os.Stderr, "%s sms send failed: %v\n", n.Name(), err)
				}
			}
		}
	}
}

func openSerialPort(portName string, baud int) (serial.Port, error) {
	if baud <= 0 {
		return nil, fmt.Errorf("invalid baud %d", baud)
	}
	return serial.Open(portName, &serial.Mode{
		BaudRate: baud,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	})
}

func initAir780E(modem *modemat.AT) error {
	commands := []string{
		"",
		"E0",
		"+CPIN?",
		"+CSQ",
		"+CMGF=1",
		"+CNMI=2,2,0,0,0",
	}

	for _, cmd := range commands {
		if _, err := runATCommand(modem, cmd); err != nil {
			return err
		}
	}
	return nil
}

func runATCommand(modem *modemat.AT, cmd string) ([]string, error) {
	display := "AT" + cmd
	fmt.Printf("TX %s\n", display)
	info, err := modem.Command(cmd)
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", display, err)
	}
	for _, line := range info {
		fmt.Printf("RX %s\n", line)
	}
	return info, nil
}
