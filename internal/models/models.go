// Package models, GoPulse genelinde paylaşılan çekirdek veri yapılarını
// tanımlar. Bu tipler katmanlar arası (checker, database, web) ortak dildir.
package models

import "time"

// MonitorType, bir izlemenin türünü belirtir (http, tcp, ...).
// Checker registry'sindeki anahtarlarla eşleşir.
type MonitorType string

const (
	MonitorHTTP MonitorType = "http"
	MonitorTCP  MonitorType = "tcp"
)

// Status, bir kontrol sonucundaki erişilebilirlik durumudur.
type Status string

const (
	StatusUp      Status = "up"      // Hedef erişilebilir.
	StatusDown    Status = "down"    // Hedef erişilemez.
	StatusPending Status = "pending" // Henüz kontrol edilmedi.
)

// Monitor, izlenen tek bir hedefi temsil eder.
type Monitor struct {
	ID        int64
	Name      string
	Type      MonitorType
	Target    string        // URL (http) veya host:port (tcp).
	Interval  time.Duration // Kontrol aralığı.
	Enabled   bool
	CreatedAt time.Time
}

// CheckResult, bir izlemenin tek bir kontrol çalışmasının kaydıdır.
// Pruning mekanizması bu tablodaki eski kayıtları budar.
type CheckResult struct {
	ID        int64
	MonitorID int64
	Status    Status
	Latency   time.Duration // Yanıt süresi.
	Message   string        // Hata/bilgi mesajı.
	CheckedAt time.Time
}

// User, panele giriş yapabilen yetkili kullanıcıdır
// (Multi-User, Single-Tenant).
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

// NotificationChannel, bir bildirim alıcısını/kanalını temsil eder
// (Telegram sohbeti, e-posta adresi vb.). Panelden dinamik yönetilir.
type NotificationChannel struct {
	ID      int64
	Type    string // "telegram", "smtp" — Notifier registry anahtarı.
	Label   string
	Config  string // Kanala özel ayarlar (JSON olarak saklanır).
	Enabled bool
}
