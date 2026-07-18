package notifier

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
)

// SMTPNotifier, bir SMTP sunucusu üzerinden e-posta bildirimi gönderir.
type SMTPNotifier struct {
	host     string
	port     int
	username string
	password string
	from     string
	to       []string
}

func (s *SMTPNotifier) Type() string { return TypeSMTP }

// Send, mesajı RFC 5322 biçiminde bir e-posta olarak gönderir.
// Not: net/smtp context'i desteklemez; ctx yalnızca iptal bilgisi için alınır.
func (s *SMTPNotifier) Send(ctx context.Context, msg Message) error {
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))

	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	body := buildEmail(s.from, s.to, msg)

	if err := smtp.SendMail(addr, auth, s.from, s.to, body); err != nil {
		return fmt.Errorf("smtp gönderimi başarısız: %w", err)
	}
	return nil
}

// buildEmail, basit bir düz metin e-posta (başlıklar + gövde) oluşturur.
func buildEmail(from string, to []string, msg Message) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	b.WriteString("Subject: " + msg.Title + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(msg.Body)
	b.WriteString("\r\n")
	return []byte(b.String())
}
