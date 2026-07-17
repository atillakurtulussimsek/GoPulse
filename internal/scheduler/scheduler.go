// Package scheduler, aktif monitörleri kendi kontrol aralıklarına göre
// periyodik olarak çalıştıran zamanlayıcıdır. Mimari: bir dispatcher sırası
// gelmiş monitörleri tespit edip iş kuyruğuna atar; sabit sayıda worker bu
// işleri eşzamanlı olarak yürütür (worker pool). Her kontrol sonucu
// check_results tablosuna yazılır ve durum değişiklikleri tespit edilir.
package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/checker"
	"github.com/atillakurtulussimsek/GoPulse/internal/config"
	"github.com/atillakurtulussimsek/GoPulse/internal/database"
	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// Scheduler, izleme çalıştırma döngüsünü yönetir.
type Scheduler struct {
	store    *database.Store
	registry *checker.Registry

	workers          int
	checkTimeout     time.Duration
	dispatchInterval time.Duration
	defaultInterval  time.Duration

	jobs chan models.Monitor

	// nextRun, her monitörün bir sonraki kontrol zamanını tutar. Yalnızca
	// dispatch goroutine'i tarafından okunup yazılır; bu yüzden kilit gerekmez.
	nextRun map[int64]time.Time
}

// New, verilen bağımlılıklar ve konfigürasyonla bir Scheduler oluşturur.
func New(store *database.Store, registry *checker.Registry, cfg config.Config) *Scheduler {
	workers := cfg.Workers
	if workers < 1 {
		workers = 1
	}
	return &Scheduler{
		store:            store,
		registry:         registry,
		workers:          workers,
		checkTimeout:     cfg.CheckTimeout,
		dispatchInterval: cfg.DispatchInterval,
		defaultInterval:  cfg.DefaultInterval,
		jobs:             make(chan models.Monitor, workers),
		nextRun:          make(map[int64]time.Time),
	}
}

// Run, worker havuzunu ve dispatch döngüsünü başlatır. ctx iptal edilene
// kadar bloklar; bu yüzden ayrı bir goroutine içinde çağrılması beklenir.
// Kapanışta iş kuyruğunu kapatır ve tüm worker'ların bitmesini bekler.
func (s *Scheduler) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < s.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for m := range s.jobs {
				s.processMonitor(ctx, m)
			}
		}()
	}

	ticker := time.NewTicker(s.dispatchInterval)
	defer ticker.Stop()

	// Açılışta hemen bir tur çalıştır (tüm aktif monitörler due sayılır).
	s.dispatchDue(ctx, time.Now())

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case now := <-ticker.C:
			s.dispatchDue(ctx, now)
		}
	}

	close(s.jobs)
	wg.Wait()
}

// dispatchDue, sırası gelmiş (next_run <= now) aktif monitörleri iş kuyruğuna
// atar ve bir sonraki çalışma zamanlarını günceller.
func (s *Scheduler) dispatchDue(ctx context.Context, now time.Time) {
	monitors, err := s.store.ListMonitors()
	if err != nil {
		log.Printf("scheduler: monitörler listelenemedi: %v", err)
		return
	}

	for _, m := range monitors {
		if !m.Enabled {
			continue
		}
		if next, seen := s.nextRun[m.ID]; seen && now.Before(next) {
			continue // Henüz sırası gelmedi.
		}

		interval := m.Interval
		if interval <= 0 {
			interval = s.defaultInterval
		}
		s.nextRun[m.ID] = now.Add(interval)

		select {
		case s.jobs <- m:
		case <-ctx.Done():
			return
		}
	}
}

// processMonitor, tek bir monitörü kontrol eder, sonucu kaydeder ve durum
// değişikliğini tespit eder. Monitör başına seri çalışır (aynı monitör aynı
// anda iki worker'a düşmez), bu yüzden önceki durumu DB'den okumak güvenlidir.
func (s *Scheduler) processMonitor(ctx context.Context, m models.Monitor) {
	chk, err := s.registry.Get(m.Type)
	if err != nil {
		log.Printf("scheduler: monitor %d (%s) için checker bulunamadı: %v", m.ID, m.Type, err)
		return
	}

	// Durum değişikliği tespiti için önceki (en son) durumu al.
	var prevStatus models.Status
	if prev, err := s.store.ListResultsByMonitor(m.ID, 1); err == nil && len(prev) > 0 {
		prevStatus = prev[0].Status
	}

	// Kontrolü zaman aşımı bağlamı altında çalıştır.
	checkCtx, cancel := context.WithTimeout(ctx, s.checkTimeout)
	res := chk.Check(checkCtx, m.Target)
	cancel()

	if _, err := s.store.InsertCheckResult(models.CheckResult{
		MonitorID: m.ID,
		Status:    res.Status,
		Latency:   res.Latency,
		Message:   res.Message,
		CheckedAt: time.Now().UTC(),
	}); err != nil {
		log.Printf("scheduler: monitor %d sonucu yazılamadı: %v", m.ID, err)
		return
	}

	// İlk kontrol (prevStatus boş) bir değişiklik sayılmaz; gürültü üretmez.
	if prevStatus != "" && prevStatus != res.Status {
		log.Printf("scheduler: durum değişikliği — %q (id=%d): %s → %s (%s)",
			m.Name, m.ID, prevStatus, res.Status, res.Message)
		// TODO(notifier): durum değişikliğinde ilgili bildirim kanalları
		// tetiklenecek (Notifier milestone'unda bağlanacak).
	}
}
