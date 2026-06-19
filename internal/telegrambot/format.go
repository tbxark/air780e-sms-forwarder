package telegrambot

import (
	"fmt"
	"html"
	"strconv"
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

func explainCommandResult(result commandResult) []string {
	if result.Err != nil {
		return []string{fmt.Sprintf("Execution failed: <code>%s</code>", html.EscapeString(result.Err.Error()))}
	}
	if hasErrorLine(result.Lines) {
		return []string{"The module returned ERROR; the command did not complete successfully."}
	}

	switch result.Command {
	case "+CPIN?":
		return explainCPIN(result.Lines)
	case "+CSQ":
		return explainCSQ(result.Lines)
	case "+CREG?", "+CEREG?":
		return explainRegistration(result.Command, result.Lines)
	case "+COPS?":
		return explainCOPS(result.Lines)
	case "+CFUN?":
		return explainCFUN(result.Lines)
	case "+CCID":
		return explainSingleValue("ICCID", result.Lines)
	case "+CGMI":
		return explainSingleValue("Manufacturer", result.Lines)
	case "+CGMM":
		return explainSingleValue("Model", result.Lines)
	case "+CGMR":
		return explainSingleValue("Firmware version", result.Lines)
	case "+CGSN":
		return explainSingleValue("IMEI/serial number", result.Lines)
	case "+CMGF=1":
		return []string{"SMS mode has been switched to text mode."}
	case "+CNMI=2,2,0,0,0":
		return []string{"New SMS push has been configured to report directly over the serial port."}
	case "+CPMS?":
		return explainCPMS(result.Lines)
	case "+CMGL=\"REC UNREAD\"", "+CMGL=\"ALL\"":
		return explainCMGL(result.Lines)
	case "+RESET":
		return []string{"Restart command sent. The serial connection may drop briefly while the module reboots."}
	default:
		if okOnly(result.Lines) {
			return []string{"Command completed successfully."}
		}
		return []string{"No structured explanation is available for this response yet."}
	}
}

func explainCPIN(lines []string) []string {
	value := prefixedValue(lines, "+CPIN:")
	if value == "" {
		return []string{"No SIM status was read."}
	}
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "READY":
		return []string{"SIM status: ready."}
	case "SIM PIN":
		return []string{"SIM status: PIN unlock required."}
	case "SIM PUK":
		return []string{"SIM status: PUK unlock required."}
	default:
		return []string{fmt.Sprintf("SIM status: <code>%s</code>.", html.EscapeString(value))}
	}
}

func explainCSQ(lines []string) []string {
	values := prefixedCSV(lines, "+CSQ:")
	if len(values) < 2 {
		return []string{"No signal quality was read."}
	}
	rssi, _ := strconv.Atoi(values[0])
	ber := strings.TrimSpace(values[1])
	return []string{
		fmt.Sprintf("Signal strength: %s.", html.EscapeString(formatRSSI(rssi))),
		fmt.Sprintf("Bit error rate (BER): %s.", html.EscapeString(formatBER(ber))),
	}
}

func explainRegistration(command string, lines []string) []string {
	prefix := strings.TrimSuffix(command, "?") + ":"
	values := prefixedCSV(lines, prefix)
	if len(values) == 0 {
		return []string{"No network registration status was read."}
	}
	stat := values[0]
	if len(values) > 1 {
		stat = values[1]
	}
	name := "2G/3G network registration"
	if command == "+CEREG?" {
		name = "EPS/LTE network registration"
	}
	return []string{fmt.Sprintf("%s: %s.", name, html.EscapeString(registrationStatus(stat)))}
}

func explainCOPS(lines []string) []string {
	values := prefixedCSV(lines, "+COPS:")
	if len(values) < 3 {
		return []string{"No operator information was read."}
	}
	items := []string{
		fmt.Sprintf("Network selection mode: %s.", html.EscapeString(operatorMode(values[0]))),
		fmt.Sprintf("Operator: <code>%s</code>.", html.EscapeString(unquote(values[2]))),
	}
	if len(values) > 3 {
		items = append(items, fmt.Sprintf("Access technology: %s.", html.EscapeString(accessTechnology(values[3]))))
	}
	return items
}

func explainCFUN(lines []string) []string {
	values := prefixedCSV(lines, "+CFUN:")
	if len(values) == 0 {
		return []string{"No function mode was read."}
	}
	return []string{fmt.Sprintf("Function mode: %s.", html.EscapeString(functionMode(values[0])))}
}

func explainSingleValue(label string, lines []string) []string {
	for _, line := range responseLines(lines) {
		return []string{fmt.Sprintf("%s: <code>%s</code>.", html.EscapeString(label), html.EscapeString(line))}
	}
	return []string{fmt.Sprintf("No %s was read.", html.EscapeString(label))}
}

func explainCPMS(lines []string) []string {
	values := prefixedCSV(lines, "+CPMS:")
	if len(values) < 3 {
		return []string{"No SMS storage information was read."}
	}
	items := make([]string, 0, 3)
	labels := []string{"Read/delete storage", "Write/send storage", "Receive storage"}
	for i := 0; i+2 < len(values) && i/3 < len(labels); i += 3 {
		items = append(items, fmt.Sprintf("%s: <code>%s</code>, used %s / total %s.", labels[i/3], html.EscapeString(unquote(values[i])), html.EscapeString(values[i+1]), html.EscapeString(values[i+2])))
	}
	return items
}

func explainCMGL(lines []string) []string {
	messages := parseCMGL(lines)
	if len(messages) == 0 {
		return []string{"No SMS messages matched the query."}
	}
	items := []string{fmt.Sprintf("Read %d SMS message(s).", len(messages))}
	for _, msg := range messages {
		items = append(items, fmt.Sprintf("#%s %s from <code>%s</code> at <code>%s</code>\n<pre>%s</pre>", html.EscapeString(msg.index), html.EscapeString(msg.status), html.EscapeString(msg.from), html.EscapeString(defaultText(msg.at, "unknown")), escapeAndTruncate(defaultText(msg.text, "(empty)"), 600)))
	}
	return items
}

func formatRSSI(v int) string {
	switch {
	case v == 99:
		return "unknown"
	case v == 0:
		return "very weak, about -113 dBm or lower"
	case v == 1:
		return "very weak, about -111 dBm"
	case v >= 2 && v <= 30:
		dbm := -113 + 2*v
		level := "weak"
		if v >= 20 {
			level = "good"
		} else if v >= 10 {
			level = "fair"
		}
		return fmt.Sprintf("%s, about %d dBm", level, dbm)
	case v == 31:
		return "excellent, about -51 dBm or higher"
	default:
		return fmt.Sprintf("unknown value %d", v)
	}
}

func formatBER(value string) string {
	switch strings.TrimSpace(value) {
	case "99":
		return "unknown or not detectable"
	case "0":
		return "lowest"
	case "1", "2":
		return "low"
	case "3", "4":
		return "medium"
	case "5", "6", "7":
		return "high, link quality may be poor"
	default:
		return "unknown value " + value
	}
}

func registrationStatus(value string) string {
	switch strings.TrimSpace(value) {
	case "0":
		return "not registered, not searching"
	case "1":
		return "registered on home network"
	case "2":
		return "not registered, searching"
	case "3":
		return "registration denied"
	case "4":
		return "unknown status"
	case "5":
		return "registered while roaming"
	default:
		return "unknown value " + value
	}
}

func operatorMode(value string) string {
	switch strings.TrimSpace(value) {
	case "0":
		return "automatic selection"
	case "1":
		return "manual selection"
	case "2":
		return "deregistered from network"
	case "3":
		return "set operator format only"
	case "4":
		return "manual first, automatic fallback"
	default:
		return "unknown value " + value
	}
}

func accessTechnology(value string) string {
	switch strings.TrimSpace(value) {
	case "0":
		return "GSM"
	case "2":
		return "UTRAN/3G"
	case "7":
		return "LTE/E-UTRAN"
	case "8":
		return "EC-GSM-IoT"
	case "9":
		return "NB-IoT"
	default:
		return "unknown value " + value
	}
}

func functionMode(value string) string {
	switch strings.TrimSpace(value) {
	case "0":
		return "minimum functionality"
	case "1":
		return "full functionality"
	case "4":
		return "airplane mode / RF disabled"
	default:
		return "unknown value " + value
	}
}

func prefixedValue(lines []string, prefix string) string {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func prefixedCSV(lines []string, prefix string) []string {
	value := prefixedValue(lines, prefix)
	if value == "" {
		return nil
	}
	return splitCSV(value)
}

func splitCSV(value string) []string {
	var fields []string
	var b strings.Builder
	inQuote := false
	for _, r := range value {
		switch r {
		case '"':
			inQuote = !inQuote
			b.WriteRune(r)
		case ',':
			if inQuote {
				b.WriteRune(r)
				continue
			}
			fields = append(fields, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	fields = append(fields, strings.TrimSpace(b.String()))
	return fields
}

func responseLines(lines []string) []string {
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "OK" || line == "ERROR" {
			continue
		}
		clean = append(clean, line)
	}
	return clean
}

func hasErrorLine(lines []string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) == "ERROR" {
			return true
		}
	}
	return false
}

func okOnly(lines []string) bool {
	seenOK := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line != "OK" {
			return false
		}
		seenOK = true
	}
	return seenOK
}

func formatRawLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return escapeAndTruncate(strings.Join(lines, "\n"), 900)
}

func unquote(value string) string {
	return strings.Trim(strings.TrimSpace(value), "\"")
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

type listedSMS struct {
	index  string
	status string
	from   string
	at     string
	text   string
}

func parseCMGL(lines []string) []listedSMS {
	var messages []listedSMS
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "+CMGL:") {
			continue
		}
		values := splitCSV(strings.TrimSpace(strings.TrimPrefix(line, "+CMGL:")))
		msg := listedSMS{index: "?", status: "unknown", from: "unknown"}
		if len(values) > 0 {
			msg.index = values[0]
		}
		if len(values) > 1 {
			msg.status = unquote(values[1])
		}
		if len(values) > 2 {
			msg.from = unquote(values[2])
		}
		if len(values) > 4 {
			msg.at = unquote(values[4])
		}
		if i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			if next != "" && !strings.HasPrefix(next, "+") && next != "OK" && next != "ERROR" {
				msg.text = next
			}
		}
		messages = append(messages, msg)
	}
	return messages
}
