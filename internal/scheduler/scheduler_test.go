package scheduler

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/checker"
	"github.com/atillakurtulussimsek/GoPulse/internal/config"
	"github.com/atillakurtulussimsek/GoPulse/internal/database"
	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// fakeChecker, testlerde deterministik sonuç döndüren sahte bir checker'dır.
type fakeChecker struct {
	typ    models.MonitorType
	result checker.Result
	calls  int
}

func (f *fakeChecker) Type() models.MonitorType { return f.typ }

func (f *fakeChecker) Check(ctx context.Context, target string) checker.Result {
	f.calls++
	return f.result
}

// newTestScheduler, geçici DB ve sahte checker ile bir scheduler kurar.
func newTestScheduler(t *testing.T, fc *fakeChecker) (*Scheduler, *database.Store) {
	t.Helper()
	store, err := database.Open(filepath.Join(t.TempDir(), "sched.db"))
	if err != nil {
		t.Fatalf("DB açılamadı: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	reg := checker.NewRegistry()
	reg.Register(fc)

	cfg := config.Config{
		Workers:          2,
		CheckTimeout:     time.Second,
		DispatchInterval: 10 * time.Millisecond,
		DefaultInterval:  time.Minute,
	}
	return New(store, reg, cfg, nil), store
}

// fakeHandler, durum değişikliği çağrılarını kaydeder.
type fakeHandler struct {
	mu    sync.Mutex
	calls []string
}

func (h *fakeHandler) OnStatusChange(m models.Monitor, prev, curr models.Status, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, string(prev)+"->"+string(curr))
}

func (h *fakeHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.calls)
}

// TestStatusChangeTriggersHandler, durum değişiminde handler'ın çağrıldığını
// ve ilk kontrolün (önceki durum yok) çağrı üretmediğini doğrular.
func TestStatusChangeTriggersHandler(t *testing.T) {
	fc := &fakeChecker{typ: "fake", result: checker.Result{Status: models.StatusUp, Message: "ok"}}
	h := &fakeHandler{}
	s, store := newTestScheduler(t, fc)
	s.onChange = h

	id, _ := store.CreateMonitor(models.Monitor{
		Name: "M", Type: "fake", Target: "x", Interval: time.Minute, Enabled: true,
	})
	m := models.Monitor{ID: id, Name: "M", Type: "fake", Target: "x", Interval: time.Minute, Enabled: true}

	// 1. kontrol: önceki durum yok → handler çağrılmamalı.
	s.processMonitor(context.Background(), m)
	// 2. kontrol: aynı durum (up) → değişiklik yok.
	s.processMonitor(context.Background(), m)
	// 3. kontrol: down'a geç → handler çağrılmalı.
	fc.result = checker.Result{Status: models.StatusDown, Message: "hata"}
	s.processMonitor(context.Background(), m)

	// Handler ayrı goroutine'de çağrıldığı için kısa bekle.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && h.count() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if got := h.count(); got != 1 {
		t.Fatalf("tam olarak 1 durum değişikliği bekleniyordu, gelen %d (%v)", got, h.calls)
	}
}

// TestProcessMonitorWritesResult, processMonitor'ın kontrol sonucunu DB'ye
// yazdığını doğrular.
func TestProcessMonitorWritesResult(t *testing.T) {
	fc := &fakeChecker{
		typ:    "fake",
		result: checker.Result{Status: models.StatusUp, Latency: 42 * time.Millisecond, Message: "ok"},
	}
	s, store := newTestScheduler(t, fc)

	id, err := store.CreateMonitor(models.Monitor{
		Name: "Test", Type: "fake", Target: "x", Interval: time.Minute, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateMonitor: %v", err)
	}

	s.processMonitor(context.Background(), models.Monitor{
		ID: id, Name: "Test", Type: "fake", Target: "x", Interval: time.Minute, Enabled: true,
	})

	if fc.calls != 1 {
		t.Fatalf("checker 1 kez çağrılmalı, çağrılan %d", fc.calls)
	}

	results, err := store.ListResultsByMonitor(id, 10)
	if err != nil {
		t.Fatalf("ListResultsByMonitor: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("1 sonuç bekleniyor, bulunan %d", len(results))
	}
	if results[0].Status != models.StatusUp || results[0].Latency != 42*time.Millisecond {
		t.Fatalf("sonuç alanları hatalı: %+v", results[0])
	}
}

// TestRunDispatchesEnabledMonitors, Run döngüsünün aktif bir monitörü
// çalıştırıp sonuç yazdığını, devre dışı monitörü atladığını doğrular.
func TestRunDispatchesEnabledMonitors(t *testing.T) {
	fc := &fakeChecker{
		typ:    "fake",
		result: checker.Result{Status: models.StatusUp, Message: "ok"},
	}
	s, store := newTestScheduler(t, fc)

	enabledID, _ := store.CreateMonitor(models.Monitor{
		Name: "Aktif", Type: "fake", Target: "a", Interval: time.Minute, Enabled: true,
	})
	disabledID, _ := store.CreateMonitor(models.Monitor{
		Name: "Pasif", Type: "fake", Target: "b", Interval: time.Minute, Enabled: false,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	// Açılış turunun aktif monitörü işlemesi için kısa süre bekle.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r, _ := store.ListResultsByMonitor(enabledID, 1); len(r) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler zamanında durmadı")
	}

	enabledResults, _ := store.ListResultsByMonitor(enabledID, 10)
	if len(enabledResults) == 0 {
		t.Fatal("aktif monitör için sonuç yazılmalıydı")
	}
	disabledResults, _ := store.ListResultsByMonitor(disabledID, 10)
	if len(disabledResults) != 0 {
		t.Fatalf("pasif monitör çalıştırılmamalı, bulunan %d sonuç", len(disabledResults))
	}
}
