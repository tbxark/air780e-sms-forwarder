package modem

import (
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/tbxark/air780e-sms-forwarder/internal/sms"
	modemat "github.com/warthog618/modem/at"
)

func NewAT(port io.ReadWriter, rawLines chan<- string, events chan<- sms.Event) *modemat.AT {
	return modemat.New(port,
		modemat.WithTimeout(5*time.Second),
		modemat.WithIndication("+CMT:", func(lines []string) {
			emitRawLines(rawLines, lines)
			event, err := sms.ParseCMTIndication(lines)
			if err != nil {
				slog.Error("parse +CMT failed", "err", err)
				return
			}
			emitSMSEvent(events, event)
		}, modemat.WithTrailingLine),
		modemat.WithIndication("+CMTI:", func(lines []string) {
			emitRawLines(rawLines, lines)
		}),
		modemat.WithIndication("+CDS:", func(lines []string) {
			emitRawLines(rawLines, lines)
		}, modemat.WithTrailingLine),
	)
}

func InitAir780E(modem *modemat.AT) error {
	commands := []string{
		"",
		"E0",
		"+CPIN?",
		"+CSQ",
		"+CMGF=1",
		"+CNMI=2,2,0,0,0",
	}

	for _, cmd := range commands {
		if _, err := RunATCommand(modem, cmd); err != nil {
			return err
		}
	}
	return nil
}

func RunATCommand(modem *modemat.AT, cmd string) ([]string, error) {
	display := "AT" + cmd
	slog.Info("at command tx", "command", display)
	info, err := modem.Command(cmd)
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", display, err)
	}
	for _, line := range info {
		slog.Info("at command rx", "line", line)
	}
	return info, nil
}

func emitRawLines(rawLines chan<- string, lines []string) {
	for _, line := range lines {
		select {
		case rawLines <- line:
		default:
			slog.Warn("raw line dropped", "line", line)
		}
	}
}

func emitSMSEvent(events chan<- sms.Event, event sms.Event) {
	select {
	case events <- event:
	default:
		slog.Warn("sms event dropped", "from", event.From)
	}
}
