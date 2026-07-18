package notifier

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/database"
	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

func TestBuildValidation(t *testing.T) {
	// Geçerli telegram.
	n, err := Build(models.NotificationChannel{
		Type: TypeTelegram, Config: `{"token":"abc","chat_id":"123"}`,
	}, nil)
	if err != nil {
		t.Fatalf("geçerli telegram build hata verdi: %v", err)
	}
	if n.Type() != TypeTelegram {
		t.Fatalf("telegram bekleniyordu, gelen %s", n.Type())
	}

	// Eksik token.
	if _, err := Build(models.NotificationChannel{Type: TypeTelegram, Config: `{"chat_id":"1"}`}, nil); err == nil {
		t.Fatal("eksik token hata vermeliydi")
	}
	// Eksik SMTP alanları.
	if _, err := Build(models.NotificationChannel{Type: TypeSMTP, Config: `{"host":"x"}`}, nil); err == nil {
		t.Fatal("eksik smtp alanları hata vermeliydi")
	}
	// Bilinmeyen tür.
	if _, err := Build(models.NotificationChannel{Type: "sms"}, nil); err == nil {
		t.Fatal("bilinmeyen tür hata vermeliydi")
	}
}

func TestTelegramSend(t *testing.T) {
	var gotChatID, gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotChatID = r.FormValue("chat_id")
		gotText = r.FormValue("text")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tg := &TelegramNotifier{token: "tok", chatID: "555", client: srv.Client(), apiBase: srv.URL}
	err := tg.Send(context.Background(), Message{Title: "Başlık", Body: "Gövde"})
	if err != nil {
		t.Fatalf("Send hata verdi: %v", err)
	}
	if gotChatID != "555" {
		t.Fatalf("chat_id 555 bekleniyordu, gelen %q", gotChatID)
	}
	if gotText != "Başlık\n\nGövde" {
		t.Fatalf("metin hatalı, gelen %q", gotText)
	}
}

func TestTelegramSendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false}`))
	}))
	defer srv.Close()

	tg := &TelegramNotifier{token: "tok", chatID: "1", client: srv.Client(), apiBase: srv.URL}
	if err := tg.Send(context.Background(), Message{Title: "x"}); err == nil {
		t.Fatal("400 yanıtı hata vermeliydi")
	}
}

func TestDispatcherOnStatusChange(t *testing.T) {
	received := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		received <- r.FormValue("text")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Telegram API kökünü test sunucusuna yönlendir.
	old := defaultTelegramAPIBase
	defaultTelegramAPIBase = srv.URL
	defer func() { defaultTelegramAPIBase = old }()

	store, err := database.Open(filepath.Join(t.TempDir(), "n.db"))
	if err != nil {
		t.Fatalf("DB: %v", err)
	}
	defer store.Close()

	mID, _ := store.CreateMonitor(models.Monitor{
		Name: "API", Type: models.MonitorHTTP, Target: "https://api", Interval: time.Minute, Enabled: true,
	})
	cID, _ := store.CreateChannel(models.NotificationChannel{
		Type: TypeTelegram, Label: "t", Config: `{"token":"tok","chat_id":"9"}`, Enabled: true,
	})
	_ = store.SetMonitorChannels(mID, []int64{cID})

	d := NewDispatcher(store, 5*time.Second)
	m, _, _ := getMonitor(t, store, mID)
	d.OnStatusChange(m, models.StatusUp, models.StatusDown, "bağlantı hatası")

	select {
	case text := <-received:
		if text == "" {
			t.Fatal("boş bildirim metni")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("bildirim gönderilmedi")
	}
}

// getMonitor, testte monitörü listeden bulur.
func getMonitor(t *testing.T, store *database.Store, id int64) (models.Monitor, bool, error) {
	t.Helper()
	monitors, err := store.ListMonitors()
	if err != nil {
		return models.Monitor{}, false, err
	}
	for _, m := range monitors {
		if m.ID == id {
			return m, true, nil
		}
	}
	return models.Monitor{}, false, nil
}
