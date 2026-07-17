// Package notifier, bildirim kanallarının genişletilebilir çekirdeğini
// tanımlar. Her kanal (Telegram, SMTP) Notifier interface'ini uygular;
// yeni kanal eklemek çekirdeği değiştirmez (Strateji deseni).
package notifier

import "context"

// Message, bir izleme durum değişikliğinde gönderilecek bildirimdir.
type Message struct {
	// İnsan okunur başlık (örn. "GoPulse uyarısı: Örnek Site DOWN").
	Title string
	// Bildirim gövdesi (durum, hedef, zaman vb.).
	Body string
}

// Notifier, tek bir bildirim kanalının gönderim mantığını soyutlar.
type Notifier interface {
	// Type, bu notifier'ın kanal türünü döndürür (registry anahtarı).
	Type() string

	// Send, mesajı ilgili kanala gönderir. ctx iptali/timeout'una saygı
	// gösterilmelidir.
	Send(ctx context.Context, msg Message) error
}
