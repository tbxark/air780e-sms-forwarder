package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type notifier interface {
	Name() string
	SendSMS(context.Context, SMSEvent) error
	SendRaw(context.Context, string) error
}

func buildNotifiers(cfg Config) []notifier {
	var notifiers []notifier
	if cfg.TelegramToken != "" && cfg.TelegramChat != "" {
		notifiers = append(notifiers, &telegramNotifier{
			token:  cfg.TelegramToken,
			chatID: cfg.TelegramChat,
			http:   &http.Client{Timeout: 15 * time.Second},
		})
	}
	return notifiers
}

type telegramNotifier struct {
	token  string
	chatID string
	http   *http.Client
}

func (t *telegramNotifier) Name() string {
	return "Telegram"
}

func (t *telegramNotifier) SendSMS(ctx context.Context, sms SMSEvent) error {
	return t.sendText(ctx, fmt.Sprintf("SMS from %s\n%s", sms.From, sms.Text))
}

func (t *telegramNotifier) SendRaw(ctx context.Context, line string) error {
	return t.sendText(ctx, "Air780E raw: "+line)
}

func (t *telegramNotifier) sendText(ctx context.Context, text string) error {
	endpoint := "https://api.telegram.org/bot" + t.token + "/sendMessage"
	body := url.Values{}
	body.Set("chat_id", t.chatID)
	body.Set("text", text)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var b bytes.Buffer
		_, _ = io.Copy(&b, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("telegram status %s: %s", resp.Status, strings.TrimSpace(b.String()))
	}

	var decoded struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return err
	}
	if !decoded.OK {
		return fmt.Errorf("telegram api error: %s", decoded.Description)
	}
	return nil
}
