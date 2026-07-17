// Package database, GoPulse'un dosya tabanlı SQLite kalıcılık katmanıdır.
// Bağlantı yönetimi, gömülü SQL migration'ları ve pruning (log budama)
// mekanizmasını içerir. CGO'suz modernc.org/sqlite sürücüsü kullanılır.
package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store, veritabanı bağlantısını ve ona bağlı işlemleri kapsüller.
type Store struct {
	db *sql.DB
}

// Open, verilen yoldaki SQLite veritabanını açar (gerekirse üst dizini
// oluşturur), önerilen pragma'ları uygular ve gömülü migration'ları
// çalıştırır.
func Open(path string) (*Store, error) {
	// Veritabanı dosyasının bulunacağı dizini garanti altına al.
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("veri dizini oluşturulamadı: %w", err)
		}
	}

	// Pragma'lar: foreign key zorunluluğu, WAL journal, makul busy timeout.
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)",
		url.PathEscape(path),
	)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("veritabanı açılamadı: %w", err)
	}

	// SQLite tek yazar destekler; bağlantı havuzunu sade tutuyoruz.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("veritabanına erişilemedi: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migration başarısız: %w", err)
	}

	return s, nil
}

// DB, alttaki *sql.DB'yi döndürür (ileri seviye/özel sorgular için).
func (s *Store) DB() *sql.DB { return s.db }

// Close, veritabanı bağlantısını kapatır.
func (s *Store) Close() error { return s.db.Close() }
