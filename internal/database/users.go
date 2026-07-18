package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// CountUsers, kayıtlı kullanıcı sayısını döndürür. İlk çalıştırma (setup)
// tespiti için kullanılır.
func (s *Store) CountUsers() (int, error) {
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&n); err != nil {
		return 0, fmt.Errorf("kullanıcı sayısı okunamadı: %w", err)
	}
	return n, nil
}

// CreateUser, verilen e-posta ve şifre hash'iyle bir kullanıcı oluşturur ve
// atanan ID'yi döndürür.
func (s *Store) CreateUser(email, passwordHash string) (int64, error) {
	res, err := s.db.Exec(
		"INSERT INTO users (email, password_hash) VALUES (?, ?)",
		email, passwordHash,
	)
	if err != nil {
		return 0, fmt.Errorf("kullanıcı oluşturulamadı: %w", err)
	}
	return res.LastInsertId()
}

// GetUserByEmail, e-postaya göre kullanıcıyı döndürür. Bulunamazsa ikinci
// dönüş değeri false olur.
func (s *Store) GetUserByEmail(email string) (models.User, bool, error) {
	var (
		u         models.User
		createdAt string
	)
	err := s.db.QueryRow(
		`SELECT id, email, password_hash, strftime('%Y-%m-%d %H:%M:%S', created_at)
		 FROM users WHERE email = ?`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.User{}, false, nil
	}
	if err != nil {
		return models.User{}, false, fmt.Errorf("kullanıcı okunamadı: %w", err)
	}
	if t, perr := parseTime(createdAt); perr == nil {
		u.CreatedAt = t
	}
	return u, true, nil
}

// CreateSession, bir oturum kaydı ekler.
func (s *Store) CreateSession(id string, userID int64, expiresAt time.Time) error {
	if _, err := s.db.Exec(
		"INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)",
		id, userID, formatTime(expiresAt),
	); err != nil {
		return fmt.Errorf("oturum oluşturulamadı: %w", err)
	}
	return nil
}

// GetSessionUser, geçerli (süresi dolmamış) bir oturuma ait kullanıcıyı
// döndürür. Oturum yok/süresi dolmuşsa ikinci dönüş değeri false olur.
func (s *Store) GetSessionUser(sessionID string, now time.Time) (models.User, bool, error) {
	var (
		u         models.User
		createdAt string
	)
	err := s.db.QueryRow(
		`SELECT u.id, u.email, u.password_hash, strftime('%Y-%m-%d %H:%M:%S', u.created_at)
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.id = ? AND s.expires_at > ?`,
		sessionID, formatTime(now),
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.User{}, false, nil
	}
	if err != nil {
		return models.User{}, false, fmt.Errorf("oturum okunamadı: %w", err)
	}
	if t, perr := parseTime(createdAt); perr == nil {
		u.CreatedAt = t
	}
	return u, true, nil
}

// DeleteSession, bir oturumu siler (çıkış / logout).
func (s *Store) DeleteSession(id string) error {
	if _, err := s.db.Exec("DELETE FROM sessions WHERE id = ?", id); err != nil {
		return fmt.Errorf("oturum silinemedi: %w", err)
	}
	return nil
}

// DeleteExpiredSessions, süresi geçmiş oturumları temizler ve silinen sayıyı
// döndürür. Pruning döngüsünde periyodik çağrılır.
func (s *Store) DeleteExpiredSessions(now time.Time) (int64, error) {
	res, err := s.db.Exec("DELETE FROM sessions WHERE expires_at <= ?", formatTime(now))
	if err != nil {
		return 0, fmt.Errorf("süresi dolmuş oturumlar silinemedi: %w", err)
	}
	return res.RowsAffected()
}
