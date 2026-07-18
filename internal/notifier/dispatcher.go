package notifier

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/database"
	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// Dispatcher, bir monitörün durum değişiminde ona bağlı aktif kanallara
// bildirim gönderir. Scheduler'ın StatusChangeHandler arayüzünü karşılar
// (yapısal uyum; scheduler paketini import etmez).
type Dispatcher struct {
	store   *database.Store
	client  *http.Client
	timeout time.Duration
}

// NewDispatcher, verilen store ve gönderim zaman aşımıyla bir Dispatcher
// oluşturur.
func NewDispatcher(store *database.Store, timeout time.Duration) *Dispatcher {
	return &Dispatcher{
		store:   store,
		client:  &http.Client{Timeout: timeout},
		timeout: timeout,
	}
}

// OnStatusChange, bir monitörün durumu değiştiğinde çağrılır. Monitöre bağlı
// aktif kanallara bildirimi gönderir. Bir kanaldaki hata diğerlerini
// etkilemez.
func (d *Dispatcher) OnStatusChange(m models.Monitor, prev, curr models.Status, message string) {
	channels, err := d.store.ListActiveChannelsForMonitor(m.ID)
	if err != nil {
		log.Printf("notifier: %q kanalları okunamadı: %v", m.Name, err)
		return
	}
	if len(channels) == 0 {
		return
	}

	msg := buildMessage(m, prev, curr, message)
	for _, ch := range channels {
		if err := d.sendTo(ch, msg); err != nil {
			log.Printf("notifier: kanal #%d (%s) gönderimi başarısız: %v", ch.ID, ch.Type, err)
		}
	}
}

// SendTest, bir kanala örnek bir test bildirimi gönderir (panel "Test" butonu).
func (d *Dispatcher) SendTest(ch models.NotificationChannel) error {
	return d.sendTo(ch, Message{
		Title: "GoPulse test bildirimi",
		Body:  "Bu bir test mesajıdır. Kanal yapılandırmanız çalışıyor. ✅",
	})
}

// sendTo, tek bir kanala mesaj gönderir.
func (d *Dispatcher) sendTo(ch models.NotificationChannel, msg Message) error {
	n, err := Build(ch, d.client)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()
	return n.Send(ctx, msg)
}

// buildMessage, durum değişikliğinden okunur bir bildirim mesajı üretir.
func buildMessage(m models.Monitor, prev, curr models.Status, detail string) Message {
	var icon, durum string
	switch curr {
	case models.StatusUp:
		icon, durum = "🟢", "ÇALIŞIYOR"
	case models.StatusDown:
		icon, durum = "🔴", "ERİŞİLEMİYOR"
	default:
		icon, durum = "⚪", string(curr)
	}

	title := fmt.Sprintf("%s GoPulse: %s %s", icon, m.Name, durum)
	body := fmt.Sprintf(
		"İzleme: %s\nHedef: %s\nDurum: %s → %s\nDetay: %s\nZaman: %s",
		m.Name, m.Target, prev, curr, detail,
		time.Now().Local().Format("2006-01-02 15:04:05"),
	)
	return Message{Title: title, Body: body}
}
