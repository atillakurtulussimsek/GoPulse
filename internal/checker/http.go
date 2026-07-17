package checker

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// HTTPChecker, bir web sitesinin HTTP/HTTPS erişilebilirliğini kontrol eder.
type HTTPChecker struct {
	client *http.Client
}

// NewHTTPChecker, verilen timeout ile bir HTTP checker oluşturur.
func NewHTTPChecker(timeout time.Duration) *HTTPChecker {
	return &HTTPChecker{
		client: &http.Client{Timeout: timeout},
	}
}

func (c *HTTPChecker) Type() models.MonitorType { return models.MonitorHTTP }

// Check, hedef URL'ye GET isteği atar. 2xx/3xx yanıtları "up" kabul edilir.
func (c *HTTPChecker) Check(ctx context.Context, target string) Result {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Result{
			Status:  models.StatusDown,
			Latency: time.Since(start),
			Message: fmt.Sprintf("geçersiz istek: %v", err),
		}
	}

	resp, err := c.client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return Result{
			Status:  models.StatusDown,
			Latency: latency,
			Message: fmt.Sprintf("bağlantı hatası: %v", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return Result{
			Status:  models.StatusUp,
			Latency: latency,
			Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}

	return Result{
		Status:  models.StatusDown,
		Latency: latency,
		Message: fmt.Sprintf("beklenmeyen durum kodu: HTTP %d", resp.StatusCode),
	}
}
