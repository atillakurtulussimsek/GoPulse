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
	"github.com/atillakurtulussimsek/GoPulse/internal/web"
)

func main() {
	cfg := config.Load()

	// Checker registry'sini kur ve mevcut izleme türlerini kaydet.
	// (Scheduler bir sonraki milestone'da bu registry'yi kullanacak.)
	registry := checker.NewRegistry()
	registry.Register(checker.NewHTTPChecker(10 * time.Second))
	registry.Register(checker.NewTCPChecker(10 * time.Second))

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
