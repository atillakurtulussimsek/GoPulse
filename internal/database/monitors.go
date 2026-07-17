package database

import (
	"fmt"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// CreateMonitor, yeni bir izleme kaydı ekler ve atanan ID'yi döndürür.
func (s *Store) CreateMonitor(m models.Monitor) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO monitors (name, type, target, interval_seconds, enabled)
		 VALUES (?, ?, ?, ?, ?)`,
		m.Name, string(m.Type), m.Target, int(m.Interval.Seconds()), boolToInt(m.Enabled),
	)
	if err != nil {
		return 0, fmt.Errorf("monitor eklenemedi: %w", err)
	}
	return res.LastInsertId()
}

// ListMonitors, tüm izlemeleri oluşturulma sırasına göre döndürür.
func (s *Store) ListMonitors() ([]models.Monitor, error) {
	rows, err := s.db.Query(
		`SELECT id, name, type, target, interval_seconds, enabled,
		        strftime('%Y-%m-%d %H:%M:%S', created_at)
		 FROM monitors
		 ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("monitorler listelenemedi: %w", err)
	}
	defer rows.Close()

	var out []models.Monitor
	for rows.Next() {
		var (
			m           models.Monitor
			typ         string
			intervalSec int
			enabled     int
			createdAt   string
		)
		if err := rows.Scan(&m.ID, &m.Name, &typ, &m.Target, &intervalSec, &enabled, &createdAt); err != nil {
			return nil, err
		}
		m.Type = models.MonitorType(typ)
		m.Interval = time.Duration(intervalSec) * time.Second
		m.Enabled = enabled != 0
		if t, err := parseTime(createdAt); err == nil {
			m.CreatedAt = t
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// boolToInt, bir boolean'ı SQLite 0/1 gösterimine çevirir.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
