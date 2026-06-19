package telegrambot

import (
	"fmt"
	"html"
	"strings"
	"time"
	"unicode/utf8"

	tgapp "github.com/go-sphere/telegram-bot/telegram"
	"github.com/go-telegram/bot/models"
	"github.com/tbxark/air780e-sms-forwarder/internal/sms"
)

const smsBodyMax = 3000

type commandResult struct {
	Command string
	Lines   []string
	Err     error
}

func formatSMSMessage(event sms.Event) *tgapp.Message {
	from := escapeAndTruncate(defaultText(event.From, "unknown"), 256)
	at := html.EscapeString(event.At.Format(time.RFC3339))
	body := escapeAndTruncate(defaultText(event.Text, "(empty)"), smsBodyMax)

	return &tgapp.Message{
		Text:      fmt.Sprintf("<b>New SMS</b>\n\n<b>From</b>: <code>%s</code>\n<b>Time</b>: <code>%s</code>\n\n<b>Content</b>\n<pre>%s</pre>", from, at, body),
		ParseMode: models.ParseModeHTML,
	}
}

func formatWatchdogAlert(reason string) *tgapp.Message {
	at := html.EscapeString(time.Now().Format(time.RFC3339))
	body := escapeAndTruncate(defaultText(reason, "serial watchdog reported an unknown error"), 1200)
	return &tgapp.Message{
		Text:      fmt.Sprintf("<b>Air780E Watchdog Alert</b>\n\n<b>Time</b>: <code>%s</code>\n<b>Reason</b>\n<pre>%s</pre>", at, body),
		ParseMode: models.ParseModeHTML,
		Button:    watchdogAlertKeyboard(),
	}
}

func formatCommandResult(_ string, results []commandResult) string {
	var b strings.Builder
	used := 0
	for _, result := range results {
		for _, line := range explainCommandResult(result) {
			if !appendHTMLChunk(&b, &used, line+"\n") {
				return strings.TrimSpace(b.String())
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func appendHTMLChunk(b *strings.Builder, used *int, chunk string) bool {
	suffix := "\n[truncated]"
	chunkLen := utf8.RuneCountInString(chunk)
	suffixLen := utf8.RuneCountInString(suffix)
	if *used+chunkLen > maxText {
		if *used+suffixLen <= maxText {
			b.WriteString(suffix)
		}
		return false
	}
	if *used+chunkLen+suffixLen > maxText {
		b.WriteString(suffix)
		*used += suffixLen
		return false
	}
	b.WriteString(chunk)
	*used += chunkLen
	return true
}

func defaultText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func escapeAndTruncate(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	suffix := "\n[truncated]"
	suffixLen := utf8.RuneCountInString(suffix)
	var b strings.Builder
	used := 0
	for i, r := range text {
		escaped := html.EscapeString(string(r))
		width := utf8.RuneCountInString(escaped)
		if i+utf8.RuneLen(r) < len(text) && used+width+suffixLen > limit {
			b.WriteString(suffix)
			return b.String()
		}
		if used+width > limit {
			b.WriteString(suffix)
			return b.String()
		}
		b.WriteString(escaped)
		used += width
	}
	return b.String()
}
