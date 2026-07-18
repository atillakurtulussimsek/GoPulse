package database

import (
	"context"
	"fmt"
	"log"
	"time"
)

// PruneOlderThan, checked_at değeri verilen cutoff zamanından eski olan tüm
// check_results kayıtlarını siler ve silinen satır sayısını döndürür.
func (s *Store) PruneOlderThan(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(
		`DELETE FROM check_results WHERE checked_at < ?`,
		formatTime(cutoff),
	)
	if err != nil {
		return 0, fmt.Errorf("pruning başarısız: %w", err)
	}
	return res.RowsAffected()
}

// Prune, verilen gün sayısından daha eski kontrol sonuçlarını siler.
// retentionDays <= 0 ise budama yapılmaz (kayıtlar süresiz saklanır).
func (s *Store) Prune(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	return s.PruneOlderThan(cutoff)
}

// StartPruningLoop, pruning'i hemen bir kez çalıştırır, ardından her
// interval'da tekrarlar. ctx iptal edilene kadar bloklar; bu yüzden
// ayrı bir goroutine içinde çağrılması beklenir.
func (s *Store) StartPruningLoop(ctx context.Context, interval time.Duration, retentionDays int) {
	runOnce := func() {
		deleted, err := s.Prune(retentionDays)
		if err != nil {
			log.Printf("pruning hatası: %v", err)
		} else if deleted > 0 {
			log.Printf("pruning: %d eski kontrol sonucu silindi", deleted)
		}

		// Süresi dolmuş oturumları da temizle.
		if sessions, err := s.DeleteExpiredSessions(time.Now().UTC()); err != nil {
			log.Printf("oturum temizleme hatası: %v", err)
		} else if sessions > 0 {
			log.Printf("pruning: %d süresi dolmuş oturum silindi", sessions)
		}
	}

	runOnce() // Açılışta bir kez.

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce()
		}
	}
}
