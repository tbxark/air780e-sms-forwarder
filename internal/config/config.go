package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const DefaultPath = "config.json"

type Config struct {
	Port                   string `json:"port"`
	Baud                   int    `json:"baud"`
	InitModem              bool   `json:"init_modem"`
	SIMReadyTimeoutSeconds int    `json:"sim_ready_timeout_seconds"`
	TelegramRaw            bool   `json:"telegram_raw"`
	TelegramToken          string `json:"telegram_token"`
	TelegramChat           string `json:"telegram_chat"`
}

func Default() Config {
	return Config{
		Port:                   "",
		Baud:                   115200,
		InitModem:              true,
		SIMReadyTimeoutSeconds: 120,
		TelegramToken:          "",
		TelegramChat:           "",
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = DefaultPath
	}

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("open config %q: %w", path, err)
	}
	defer func() {
		_ = f.Close()
	}()

	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}
	cfg.applyDefaults(Default())
	return cfg, nil
}

func (c *Config) applyDefaults(defaults Config) {
	if c.Port == "" {
		c.Port = defaults.Port
	}
	if c.Baud == 0 {
		c.Baud = defaults.Baud
	}
	if c.SIMReadyTimeoutSeconds == 0 {
		c.SIMReadyTimeoutSeconds = defaults.SIMReadyTimeoutSeconds
	}
	if c.TelegramToken == "" {
		c.TelegramToken = defaults.TelegramToken
	}
	if c.TelegramChat == "" {
		c.TelegramChat = defaults.TelegramChat
	}
}
