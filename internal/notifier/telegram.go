package notifier

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// defaultTelegramAPIBase, Telegram Bot API'sinin kök adresidir. Testlerde
// httptest sunucusuna yönlendirmek için değiştirilebilir.
var defaultTelegramAPIBase = "https://api.telegram.org"

// TelegramNotifier, bir Telegram sohbetine bot üzerinden mesaj gönderir.
type TelegramNotifier struct {
	token   string
	chatID  string
	client  *http.Client
	apiBase string
}

func (t *TelegramNotifier) Type() string { return TypeTelegram }

// Send, mesajı Telegram sendMessage API'sine gönderir.
func (t *TelegramNotifier) Send(ctx context.Context, msg Message) error {
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", t.apiBase, t.token)

	form := url.Values{
		"chat_id": {t.chatID},
		"text":    {msg.Title + "\n\n" + msg.Body},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("telegram isteği oluşturulamadı: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := t.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram gönderimi başarısız: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("telegram API hatası (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
