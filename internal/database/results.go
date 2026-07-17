package database

import (
	"fmt"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// InsertCheckResult, bir kontrol sonucunu kaydeder ve atanan ID'yi döndürür.
// CheckedAt boş bırakılırsa şu anki UTC zamanı kullanılır.
func (s *Store) InsertCheckResult(r models.CheckResult) (int64, error) {
	checkedAt := r.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	}

	res, err := s.db.Exec(
		`INSERT INTO check_results (monitor_id, status, latency_ms, message, checked_at)
		 VALUES (?, ?, ?, ?, ?)`,
		r.MonitorID, string(r.Status), r.Latency.Milliseconds(), r.Message, formatTime(checkedAt),
	)
	if err != nil {
		return 0, fmt.Errorf("kontrol sonucu eklenemedi: %w", err)
	}
	return res.LastInsertId()
}

// ListResultsByMonitor, bir izlemenin en son kontrol sonuçlarını (en yeni
// önce) verilen limite kadar döndürür.
func (s *Store) ListResultsByMonitor(monitorID int64, limit int) ([]models.CheckResult, error) {
	rows, err := s.db.Query(
		`SELECT id, monitor_id, status, latency_ms, message,
		        strftime('%Y-%m-%d %H:%M:%S', checked_at)
		 FROM check_results
		 WHERE monitor_id = ?
		 ORDER BY checked_at DESC, id DESC
		 LIMIT ?`,
		monitorID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("sonuçlar listelenemedi: %w", err)
	}
	defer rows.Close()

	var out []models.CheckResult
	for rows.Next() {
		var (
			r         models.CheckResult
			status    string
			latencyMs int64
			checkedAt string
		)
		if err := rows.Scan(&r.ID, &r.MonitorID, &status, &latencyMs, &r.Message, &checkedAt); err != nil {
			return nil, err
		}
		r.Status = models.Status(status)
		r.Latency = time.Duration(latencyMs) * time.Millisecond
		if t, err := parseTime(checkedAt); err == nil {
			r.CheckedAt = t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
