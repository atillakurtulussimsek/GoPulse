package notifier

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// Kanal türleri (models.NotificationChannel.Type ile eşleşir).
const (
	TypeTelegram = "telegram"
	TypeSMTP     = "smtp"
)

// telegramConfig, bir Telegram kanalının JSON yapılandırmasıdır.
type telegramConfig struct {
	Token  string `json:"token"`
	ChatID string `json:"chat_id"`
}

// smtpConfig, bir SMTP kanalının JSON yapılandırmasıdır.
type smtpConfig struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	From     string   `json:"from"`
	To       []string `json:"to"`
}

// Build, bir bildirim kanalı kaydından ilgili Notifier'ı oluşturur.
// Yapılandırma eksik/geçersizse hata döner.
func Build(ch models.NotificationChannel, client *http.Client) (Notifier, error) {
	switch ch.Type {
	case TypeTelegram:
		var cfg telegramConfig
		if err := json.Unmarshal([]byte(orEmptyJSON(ch.Config)), &cfg); err != nil {
			return nil, fmt.Errorf("telegram yapılandırması çözümlenemedi: %w", err)
		}
		if cfg.Token == "" || cfg.ChatID == "" {
			return nil, fmt.Errorf("telegram kanalı için token ve chat_id zorunludur")
		}
		return &TelegramNotifier{
			token:   cfg.Token,
			chatID:  cfg.ChatID,
			client:  client,
			apiBase: defaultTelegramAPIBase,
		}, nil

	case TypeSMTP:
		var cfg smtpConfig
		if err := json.Unmarshal([]byte(orEmptyJSON(ch.Config)), &cfg); err != nil {
			return nil, fmt.Errorf("smtp yapılandırması çözümlenemedi: %w", err)
		}
		if cfg.Host == "" || cfg.Port == 0 || cfg.From == "" || len(cfg.To) == 0 {
			return nil, fmt.Errorf("smtp kanalı için host, port, from ve en az bir alıcı (to) zorunludur")
		}
		return &SMTPNotifier{
			host:     cfg.Host,
			port:     cfg.Port,
			username: cfg.Username,
			password: cfg.Password,
			from:     cfg.From,
			to:       cfg.To,
		}, nil

	default:
		return nil, fmt.Errorf("bilinmeyen bildirim kanalı türü: %q", ch.Type)
	}
}

// orEmptyJSON, boş config metnini geçerli boş JSON nesnesine çevirir.
func orEmptyJSON(s string) string {
	if s == "" {
		return "{}"
	}
	return s
}
