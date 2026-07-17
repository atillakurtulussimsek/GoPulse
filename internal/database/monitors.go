package database

import (
	"database/sql"
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

// DeleteMonitor, bir izlemeyi (ve ilişkili kontrol sonuçlarını, FK CASCADE
// ile) siler.
func (s *Store) DeleteMonitor(id int64) error {
	if _, err := s.db.Exec("DELETE FROM monitors WHERE id = ?", id); err != nil {
		return fmt.Errorf("monitor silinemedi: %w", err)
	}
	return nil
}

// SetMonitorEnabled, bir izlemenin aktif/pasif durumunu günceller.
func (s *Store) SetMonitorEnabled(id int64, enabled bool) error {
	if _, err := s.db.Exec("UPDATE monitors SET enabled = ? WHERE id = ?", boolToInt(enabled), id); err != nil {
		return fmt.Errorf("monitor durumu güncellenemedi: %w", err)
	}
	return nil
}

// MonitorStatus, panelde gösterim için bir monitörü son durumu ve kısa
// istatistikleriyle birlikte taşıyan salt-okunur görünümdür.
type MonitorStatus struct {
	Monitor       models.Monitor
	HasResult     bool
	LastStatus    models.Status
	LastLatency   time.Duration
	LastCheckedAt time.Time
	// Uptime24h, son 24 saatteki 'up' oranıdır (0-100). Kayıt yoksa 0.
	Uptime24h float64
	// Total24h, son 24 saatteki toplam kontrol sayısıdır.
	Total24h int
}

// ListMonitorsWithStatus, tüm monitörleri son kontrol durumları ve son 24
// saatlik uptime oranlarıyla birlikte döndürür. cutoff, 24 saatlik pencerenin
// başlangıcıdır.
func (s *Store) ListMonitorsWithStatus(cutoff time.Time) ([]MonitorStatus, error) {
	rows, err := s.db.Query(
		`SELECT
		    m.id, m.name, m.type, m.target, m.interval_seconds, m.enabled,
		    strftime('%Y-%m-%d %H:%M:%S', m.created_at),
		    (SELECT status FROM check_results cr WHERE cr.monitor_id = m.id
		       ORDER BY cr.checked_at DESC, cr.id DESC LIMIT 1),
		    (SELECT latency_ms FROM check_results cr WHERE cr.monitor_id = m.id
		       ORDER BY cr.checked_at DESC, cr.id DESC LIMIT 1),
		    (SELECT strftime('%Y-%m-%d %H:%M:%S', cr.checked_at) FROM check_results cr
		       WHERE cr.monitor_id = m.id ORDER BY cr.checked_at DESC, cr.id DESC LIMIT 1),
		    (SELECT COUNT(*) FROM check_results cr WHERE cr.monitor_id = m.id AND cr.checked_at >= ?),
		    (SELECT COUNT(*) FROM check_results cr WHERE cr.monitor_id = m.id AND cr.checked_at >= ? AND cr.status = 'up')
		 FROM monitors m
		 ORDER BY m.id`,
		formatTime(cutoff), formatTime(cutoff),
	)
	if err != nil {
		return nil, fmt.Errorf("durum listesi alınamadı: %w", err)
	}
	defer rows.Close()

	var out []MonitorStatus
	for rows.Next() {
		var (
			m           models.Monitor
			typ         string
			intervalSec int
			enabled     int
			createdAt   string
			lastStatus  sql.NullString
			lastLatency sql.NullInt64
			lastChecked sql.NullString
			total24h    int
			up24h       int
		)
		if err := rows.Scan(
			&m.ID, &m.Name, &typ, &m.Target, &intervalSec, &enabled, &createdAt,
			&lastStatus, &lastLatency, &lastChecked, &total24h, &up24h,
		); err != nil {
			return nil, err
		}

		m.Type = models.MonitorType(typ)
		m.Interval = time.Duration(intervalSec) * time.Second
		m.Enabled = enabled != 0
		if t, err := parseTime(createdAt); err == nil {
			m.CreatedAt = t
		}

		ms := MonitorStatus{Monitor: m, Total24h: total24h}
		if lastStatus.Valid {
			ms.HasResult = true
			ms.LastStatus = models.Status(lastStatus.String)
			ms.LastLatency = time.Duration(lastLatency.Int64) * time.Millisecond
			if t, err := parseTime(lastChecked.String); err == nil {
				ms.LastCheckedAt = t
			}
		}
		if total24h > 0 {
			ms.Uptime24h = float64(up24h) / float64(total24h) * 100
		}
		out = append(out, ms)
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
