// Command gopulse, GoPulse uptime izleme botunun giriş noktasıdır.
// Bileşenleri (config, checker registry, web sunucusu) burada bağlar ve
// zarif (graceful) kapanışı yönetir.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/checker"
	"github.com/atillakurtulussimsek/GoPulse/internal/config"
	"github.com/atillakurtulussimsek/GoPulse/internal/database"
	"github.com/atillakurtulussimsek/GoPulse/internal/scheduler"
	"github.com/atillakurtulussimsek/GoPulse/internal/web"
)

func main() {
	cfg := config.Load()

	// Veritabanını aç; migration'lar otomatik uygulanır.
	store, err := database.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("veritabanı açılamadı: %v", err)
	}
	defer store.Close()
	log.Printf("veritabanı hazır: %s", cfg.DatabasePath)

	// Uygulama ömrü boyunca geçerli bağlam (pruning gibi arka plan
	// görevlerini kapanışta durdurur).
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// Periyodik pruning görevini başlat (yaş bazlı log budama).
	go store.StartPruningLoop(appCtx, cfg.PruneInterval, cfg.RetentionDays)

	// Checker registry'sini kur ve mevcut izleme türlerini kaydet.
	registry := checker.NewRegistry()
	registry.Register(checker.NewHTTPChecker(cfg.CheckTimeout))
	registry.Register(checker.NewTCPChecker(cfg.CheckTimeout))

	// Scheduler'ı başlat: aktif monitörleri aralıklarına göre çalıştırır.
	sched := scheduler.New(store, registry, cfg)
	go sched.Run(appCtx)
	log.Printf("scheduler başlatıldı: %d worker, dispatch %s", cfg.Workers, cfg.DispatchInterval)

	// Web sunucusunu oluştur.
	srv, err := web.NewServer(cfg)
	if err != nil {
		log.Fatalf("web sunucusu oluşturulamadı: %v", err)
	}

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Sunucuyu ayrı goroutine'de başlat.
	go func() {
		log.Printf("GoPulse dinlemede: %s", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("sunucu hatası: %v", err)
		}
	}()

	// SIGINT/SIGTERM ile zarif kapanış.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("kapatılıyor...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("kapatma hatası: %v", err)
	}
	log.Println("GoPulse durdu.")
}
