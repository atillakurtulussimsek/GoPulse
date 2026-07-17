package database

import "time"

// sqlTimeLayout, DATETIME sütunlarını SQLite'ın CURRENT_TIMESTAMP formatıyla
// (UTC) uyumlu biçimde yazmak/okumak için kullanılır. Metinsel karşılaştırma
// bu biçimde kronolojik sıralama ile aynıdır (pruning için önemli).
const sqlTimeLayout = "2006-01-02 15:04:05"

// formatTime, bir zamanı SQLite metin biçimine (UTC) çevirir.
func formatTime(t time.Time) string {
	return t.UTC().Format(sqlTimeLayout)
}

// parseTime, SQLite metin biçimindeki bir zamanı UTC time.Time'a çevirir.
func parseTime(s string) (time.Time, error) {
	return time.Parse(sqlTimeLayout, s)
}
