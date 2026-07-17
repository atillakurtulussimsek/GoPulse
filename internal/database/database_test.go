package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// openTestStore, testler için geçici dizinde izole bir SQLite Store açar.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open başarısız: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestMigrationIdempotent, migration'ların iki kez uygulanmasının sorun
// çıkarmadığını ve versiyonun kaydedildiğini doğrular.
func TestMigrationIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("ilk Open başarısız: %v", err)
	}

	var version int
	if err := s1.DB().QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatalf("versiyon okunamadı: %v", err)
	}
	if version != 1 {
		t.Fatalf("beklenen versiyon 1, gelen %d", version)
	}
	_ = s1.Close()

	// İkinci kez açmak migration'ları yeniden uygulamamalı (hata vermemeli).
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("ikinci Open başarısız (idempotent değil): %v", err)
	}
	defer s2.Close()

	var count int
	if err := s2.DB().QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("migration sayısı okunamadı: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration tek kez kaydedilmeli, bulunan %d", count)
	}
}

// TestMonitorCRUD, monitor oluşturma ve listelemenin verileri koruduğunu
// doğrular.
func TestMonitorCRUD(t *testing.T) {
	s := openTestStore(t)

	id, err := s.CreateMonitor(models.Monitor{
		Name:     "Örnek Site",
		Type:     models.MonitorHTTP,
		Target:   "https://example.com",
		Interval: 30 * time.Second,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("CreateMonitor başarısız: %v", err)
	}

	monitors, err := s.ListMonitors()
	if err != nil {
		t.Fatalf("ListMonitors başarısız: %v", err)
	}
	if len(monitors) != 1 {
		t.Fatalf("1 monitor bekleniyor, bulunan %d", len(monitors))
	}
	m := monitors[0]
	if m.ID != id || m.Name != "Örnek Site" || m.Type != models.MonitorHTTP {
		t.Fatalf("monitor alanları hatalı: %+v", m)
	}
	if m.Interval != 30*time.Second {
		t.Fatalf("interval 30s bekleniyor, gelen %v", m.Interval)
	}
	if !m.Enabled {
		t.Fatalf("monitor enabled olmalı")
	}
}

// TestMonitorDeleteAndToggle, silme ve aktif/pasif güncellemeyi doğrular.
func TestMonitorDeleteAndToggle(t *testing.T) {
	s := openTestStore(t)

	id, _ := s.CreateMonitor(models.Monitor{
		Name: "X", Type: models.MonitorHTTP, Target: "https://x", Interval: time.Minute, Enabled: true,
	})

	// Pasife al.
	if err := s.SetMonitorEnabled(id, false); err != nil {
		t.Fatalf("SetMonitorEnabled: %v", err)
	}
	monitors, _ := s.ListMonitors()
	if monitors[0].Enabled {
		t.Fatal("monitor pasif olmalıydı")
	}

	// Sil.
	if err := s.DeleteMonitor(id); err != nil {
		t.Fatalf("DeleteMonitor: %v", err)
	}
	monitors, _ = s.ListMonitors()
	if len(monitors) != 0 {
		t.Fatalf("monitor silinmeliydi, kalan %d", len(monitors))
	}
}

// TestListMonitorsWithStatus, durum view'inin son durumu ve 24h uptime'ı
// doğru hesapladığını doğrular.
func TestListMonitorsWithStatus(t *testing.T) {
	s := openTestStore(t)

	id, _ := s.CreateMonitor(models.Monitor{
		Name: "Site", Type: models.MonitorHTTP, Target: "https://site", Interval: time.Minute, Enabled: true,
	})

	now := time.Now().UTC()
	// Son 24 saatte 3 up, 1 down → uptime %75.
	_, _ = s.InsertCheckResult(models.CheckResult{MonitorID: id, Status: models.StatusUp, CheckedAt: now.Add(-4 * time.Hour)})
	_, _ = s.InsertCheckResult(models.CheckResult{MonitorID: id, Status: models.StatusDown, CheckedAt: now.Add(-3 * time.Hour)})
	_, _ = s.InsertCheckResult(models.CheckResult{MonitorID: id, Status: models.StatusUp, CheckedAt: now.Add(-2 * time.Hour)})
	// En son kayıt: up, 120ms.
	_, _ = s.InsertCheckResult(models.CheckResult{MonitorID: id, Status: models.StatusUp, Latency: 120 * time.Millisecond, CheckedAt: now.Add(-1 * time.Hour)})
	// 24 saatten eski kayıt → uptime hesabına girmemeli.
	_, _ = s.InsertCheckResult(models.CheckResult{MonitorID: id, Status: models.StatusDown, CheckedAt: now.Add(-30 * time.Hour)})

	list, err := s.ListMonitorsWithStatus(now.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("ListMonitorsWithStatus: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("1 monitor bekleniyor, bulunan %d", len(list))
	}
	ms := list[0]
	if !ms.HasResult || ms.LastStatus != models.StatusUp {
		t.Fatalf("son durum 'up' olmalı: %+v", ms)
	}
	if ms.LastLatency != 120*time.Millisecond {
		t.Fatalf("son latency 120ms bekleniyor, gelen %v", ms.LastLatency)
	}
	if ms.Total24h != 4 {
		t.Fatalf("24h toplam 4 bekleniyor, gelen %d", ms.Total24h)
	}
	if ms.Uptime24h != 75 {
		t.Fatalf("uptime %%75 bekleniyor, gelen %.1f", ms.Uptime24h)
	}
}

// TestPruning, yaş bazlı budamanın yalnızca eski kayıtları sildiğini
// doğrular.
func TestPruning(t *testing.T) {
	s := openTestStore(t)

	monitorID, err := s.CreateMonitor(models.Monitor{
		Name:     "Test",
		Type:     models.MonitorTCP,
		Target:   "127.0.0.1:22",
		Interval: time.Minute,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("CreateMonitor başarısız: %v", err)
	}

	now := time.Now().UTC()

	// Eski kayıt (40 gün önce) — silinmeli.
	if _, err := s.InsertCheckResult(models.CheckResult{
		MonitorID: monitorID,
		Status:    models.StatusDown,
		CheckedAt: now.AddDate(0, 0, -40),
	}); err != nil {
		t.Fatalf("eski kayıt eklenemedi: %v", err)
	}

	// Yeni kayıt (1 gün önce) — kalmalı.
	if _, err := s.InsertCheckResult(models.CheckResult{
		MonitorID: monitorID,
		Status:    models.StatusUp,
		Latency:   150 * time.Millisecond,
		CheckedAt: now.AddDate(0, 0, -1),
	}); err != nil {
		t.Fatalf("yeni kayıt eklenemedi: %v", err)
	}

	// 30 günden eski kayıtları buda.
	deleted, err := s.Prune(30)
	if err != nil {
		t.Fatalf("Prune başarısız: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("1 kayıt silinmeli, silinen %d", deleted)
	}

	remaining, err := s.ListResultsByMonitor(monitorID, 10)
	if err != nil {
		t.Fatalf("ListResultsByMonitor başarısız: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("1 kayıt kalmalı, kalan %d", len(remaining))
	}
	if remaining[0].Status != models.StatusUp {
		t.Fatalf("kalan kayıt 'up' olmalı, gelen %q", remaining[0].Status)
	}
	if remaining[0].Latency != 150*time.Millisecond {
		t.Fatalf("latency 150ms bekleniyor, gelen %v", remaining[0].Latency)
	}
}

// TestPruneDisabled, retentionDays <= 0 iken hiçbir kaydın silinmediğini
// doğrular.
func TestPruneDisabled(t *testing.T) {
	s := openTestStore(t)

	monitorID, _ := s.CreateMonitor(models.Monitor{
		Name: "X", Type: models.MonitorHTTP, Target: "https://x", Interval: time.Minute, Enabled: true,
	})
	_, _ = s.InsertCheckResult(models.CheckResult{
		MonitorID: monitorID,
		Status:    models.StatusUp,
		CheckedAt: time.Now().UTC().AddDate(0, 0, -1000),
	})

	deleted, err := s.Prune(0)
	if err != nil {
		t.Fatalf("Prune(0) hata verdi: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("budama devre dışı olmalı, silinen %d", deleted)
	}
}
