// Package checker, izleme mekanizmalarının genişletilebilir çekirdeğini
// tanımlar. Her izleme türü (HTTP, TCP, ilerde SSL/Ping) Checker
// interface'ini uygular; yeni tür eklemek çekirdeği değiştirmez
// (Strateji + Open/Closed deseni).
package checker

import (
	"context"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// Result, tek bir kontrol çalışmasının çıktısıdır.
type Result struct {
	Status  models.Status
	Latency time.Duration
	Message string
}

// Checker, tek bir izleme türünün kontrol mantığını soyutlar.
type Checker interface {
	// Type, bu checker'ın izleme türünü döndürür (registry anahtarı).
	Type() models.MonitorType

	// Check, verilen hedefi kontrol eder ve sonucu döndürür.
	// Uygulamalar ctx iptali/timeout'una saygı göstermelidir.
	Check(ctx context.Context, target string) Result
}
