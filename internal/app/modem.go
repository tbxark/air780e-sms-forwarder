package app

import (
	"fmt"
	"io"
	"os"
	"time"

	modemat "github.com/warthog618/modem/at"
)

func newATModem(port io.ReadWriter, rawLines chan<- string, events chan<- SMSEvent) *modemat.AT {
	return modemat.New(port,
		modemat.WithTimeout(5*time.Second),
		modemat.WithIndication("+CMT:", func(lines []string) {
			emitRawLines(rawLines, lines)
			sms, err := parseCMTIndication(lines)
			if err != nil {
				fmt.Fprintf(os.Stderr, "parse +CMT failed: %v\n", err)
				return
			}
			emitSMSEvent(events, sms)
		}, modemat.WithTrailingLine),
		modemat.WithIndication("+CMTI:", func(lines []string) {
			emitRawLines(rawLines, lines)
		}),
		modemat.WithIndication("+CDS:", func(lines []string) {
			emitRawLines(rawLines, lines)
		}, modemat.WithTrailingLine),
	)
}

func emitRawLines(rawLines chan<- string, lines []string) {
	for _, line := range lines {
		select {
		case rawLines <- line:
		default:
			fmt.Fprintf(os.Stderr, "raw line dropped: %s\n", line)
		}
	}
}

func emitSMSEvent(events chan<- SMSEvent, sms SMSEvent) {
	select {
	case events <- sms:
	default:
		fmt.Fprintf(os.Stderr, "sms event dropped: from=%s\n", sms.From)
	}
}
