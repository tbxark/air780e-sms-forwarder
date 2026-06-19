package app

import (
	"os"
	"strconv"
)

type Config struct {
	Port          string
	Baud          int
	ListPorts     bool
	ConfigurePort bool
	InitModem     bool
	TelegramRaw   bool
	TelegramToken string
	TelegramChat  string
}

func DefaultConfig() Config {
	return Config{
		Port:          env("AIR780E_PORT", ""),
		Baud:          envInt("AIR780E_BAUD", 115200),
		ConfigurePort: true,
		InitModem:     true,
		TelegramToken: env("TELEGRAM_BOT_TOKEN", ""),
		TelegramChat:  env("TELEGRAM_CHAT_ID", ""),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
