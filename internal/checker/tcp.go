package checker

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// TCPChecker, bir sunucudaki portun (host:port) TCP düzeyinde açık olup
// olmadığını kontrol eder.
type TCPChecker struct {
	timeout time.Duration
}

// NewTCPChecker, verilen bağlantı timeout'u ile bir TCP checker oluşturur.
func NewTCPChecker(timeout time.Duration) *TCPChecker {
	return &TCPChecker{timeout: timeout}
}

func (c *TCPChecker) Type() models.MonitorType { return models.MonitorTCP }

// Check, hedef "host:port" adresine TCP bağlantısı kurmayı dener.
// Bağlantı kurulabilirse "up", aksi halde "down".
func (c *TCPChecker) Check(ctx context.Context, target string) Result {
	start := time.Now()

	dialer := &net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", target)
	latency := time.Since(start)
	if err != nil {
		return Result{
			Status:  models.StatusDown,
			Latency: latency,
			Message: fmt.Sprintf("port kapalı/erişilemez: %v", err),
		}
	}
	_ = conn.Close()

	return Result{
		Status:  models.StatusUp,
		Latency: latency,
		Message: "port açık",
	}
}
