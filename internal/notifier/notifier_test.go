package notifier

import (
	"testing"

	"github.com/tbxark/air780e-sms-forwarder/internal/config"
)

func TestBuild(t *testing.T) {
	if got := Build(config.Config{}); len(got) != 0 {
		t.Fatalf("len = %d", len(got))
	}

	got := Build(config.Config{TelegramToken: "token", TelegramChat: "chat"})
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Name() != "Telegram" {
		t.Fatalf("name = %q", got[0].Name())
	}
}
