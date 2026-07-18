package database

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// CreateChannel, yeni bir bildirim kanalı ekler ve atanan ID'yi döndürür.
func (s *Store) CreateChannel(ch models.NotificationChannel) (int64, error) {
	config := ch.Config
	if config == "" {
		config = "{}"
	}
	res, err := s.db.Exec(
		"INSERT INTO notification_channels (type, label, config, enabled) VALUES (?, ?, ?, ?)",
		ch.Type, ch.Label, config, boolToInt(ch.Enabled),
	)
	if err != nil {
		return 0, fmt.Errorf("bildirim kanalı eklenemedi: %w", err)
	}
	return res.LastInsertId()
}

// ListChannels, tüm bildirim kanallarını ID sırasına göre döndürür.
func (s *Store) ListChannels() ([]models.NotificationChannel, error) {
	rows, err := s.db.Query(
		"SELECT id, type, label, config, enabled FROM notification_channels ORDER BY id",
	)
	if err != nil {
		return nil, fmt.Errorf("kanallar listelenemedi: %w", err)
	}
	defer rows.Close()

	var out []models.NotificationChannel
	for rows.Next() {
		ch, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// GetChannel, ID'ye göre bir kanalı döndürür. Bulunamazsa ikinci dönüş değeri
// false olur.
func (s *Store) GetChannel(id int64) (models.NotificationChannel, bool, error) {
	row := s.db.QueryRow(
		"SELECT id, type, label, config, enabled FROM notification_channels WHERE id = ?", id,
	)
	ch, err := scanChannel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return models.NotificationChannel{}, false, nil
	}
	if err != nil {
		return models.NotificationChannel{}, false, err
	}
	return ch, true, nil
}

// DeleteChannel, bir kanalı (ve monitör eşlemelerini, FK CASCADE ile) siler.
func (s *Store) DeleteChannel(id int64) error {
	if _, err := s.db.Exec("DELETE FROM notification_channels WHERE id = ?", id); err != nil {
		return fmt.Errorf("kanal silinemedi: %w", err)
	}
	return nil
}

// SetChannelEnabled, bir kanalın aktif/pasif durumunu günceller.
func (s *Store) SetChannelEnabled(id int64, enabled bool) error {
	if _, err := s.db.Exec(
		"UPDATE notification_channels SET enabled = ? WHERE id = ?", boolToInt(enabled), id,
	); err != nil {
		return fmt.Errorf("kanal durumu güncellenemedi: %w", err)
	}
	return nil
}

// SetMonitorChannels, bir monitörün bağlı olduğu kanal kümesini tümüyle
// değiştirir (önceki eşlemeleri siler, verilenleri ekler) — tek işlemde.
func (s *Store) SetMonitorChannels(monitorID int64, channelIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM monitor_channels WHERE monitor_id = ?", monitorID); err != nil {
		return fmt.Errorf("eski eşlemeler silinemedi: %w", err)
	}
	for _, cid := range channelIDs {
		if _, err := tx.Exec(
			"INSERT INTO monitor_channels (monitor_id, channel_id) VALUES (?, ?)", monitorID, cid,
		); err != nil {
			return fmt.Errorf("eşleme eklenemedi: %w", err)
		}
	}
	return tx.Commit()
}

// ListChannelIDsForMonitor, bir monitöre bağlı kanal ID'lerini döndürür.
func (s *Store) ListChannelIDsForMonitor(monitorID int64) ([]int64, error) {
	rows, err := s.db.Query(
		"SELECT channel_id FROM monitor_channels WHERE monitor_id = ? ORDER BY channel_id", monitorID,
	)
	if err != nil {
		return nil, fmt.Errorf("eşleme okunamadı: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListActiveChannelsForMonitor, bir monitöre bağlı ve aktif olan bildirim
// kanallarını döndürür. Dispatcher bildirim göndermek için bunu kullanır.
func (s *Store) ListActiveChannelsForMonitor(monitorID int64) ([]models.NotificationChannel, error) {
	rows, err := s.db.Query(
		`SELECT c.id, c.type, c.label, c.config, c.enabled
		 FROM notification_channels c
		 JOIN monitor_channels mc ON mc.channel_id = c.id
		 WHERE mc.monitor_id = ? AND c.enabled = 1
		 ORDER BY c.id`, monitorID,
	)
	if err != nil {
		return nil, fmt.Errorf("aktif kanallar okunamadı: %w", err)
	}
	defer rows.Close()

	var out []models.NotificationChannel
	for rows.Next() {
		ch, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// rowScanner, hem *sql.Row hem *sql.Rows için ortak Scan arayüzüdür.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanChannel, bir satırı NotificationChannel'a okur.
func scanChannel(sc rowScanner) (models.NotificationChannel, error) {
	var (
		ch      models.NotificationChannel
		enabled int
	)
	if err := sc.Scan(&ch.ID, &ch.Type, &ch.Label, &ch.Config, &enabled); err != nil {
		return models.NotificationChannel{}, err
	}
	ch.Enabled = enabled != 0
	return ch, nil
}
