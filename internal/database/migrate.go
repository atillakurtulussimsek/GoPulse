package database

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// migrationFS, sıralı SQL migration dosyalarını binary'ye gömer.
// Dosya adı biçimi: NNNN_aciklama.sql (örn. 0001_init.sql).
//
//go:embed migrations/*.sql
var migrationFS embed.FS

// migrate, uygulanmamış tüm migration'ları sırayla ve idempotent şekilde
// çalıştırır. Uygulanan her versiyon schema_migrations tablosuna yazılır.
func (s *Store) migrate() error {
	// Versiyon takip tablosunu garanti altına al.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
		return fmt.Errorf("schema_migrations oluşturulamadı: %w", err)
	}

	// Uygulanmış versiyonları oku.
	applied, err := s.appliedVersions()
	if err != nil {
		return err
	}

	// Gömülü migration dosyalarını topla ve sürüme göre sırala.
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("migration dosyaları okunamadı: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}

		version, err := parseVersion(e.Name())
		if err != nil {
			return err
		}
		if applied[version] {
			continue // Zaten uygulanmış.
		}

		content, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return fmt.Errorf("%s okunamadı: %w", e.Name(), err)
		}

		if err := s.applyMigration(version, string(content)); err != nil {
			return fmt.Errorf("%s uygulanamadı: %w", e.Name(), err)
		}
	}

	return nil
}

// applyMigration, tek bir migration'ı ve versiyon kaydını tek işlemde
// (transaction) uygular; hata olursa geri alınır.
func (s *Store) applyMigration(version int, sqlText string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(sqlText); err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
		return err
	}
	return tx.Commit()
}

// appliedVersions, schema_migrations tablosundaki uygulanmış versiyonları
// bir küme olarak döndürür.
func (s *Store) appliedVersions() (map[int]bool, error) {
	rows, err := s.db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// parseVersion, "0001_init.sql" biçimindeki dosya adından sayısal versiyonu
// çıkarır.
func parseVersion(name string) (int, error) {
	prefix, _, found := strings.Cut(name, "_")
	if !found {
		return 0, fmt.Errorf("geçersiz migration adı (NNNN_ad.sql bekleniyor): %q", name)
	}
	v, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("geçersiz migration versiyonu %q: %w", name, err)
	}
	return v, nil
}
