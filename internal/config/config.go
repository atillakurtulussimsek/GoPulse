// Package config, GoPulse'un çalışma zamanı konfigürasyonunu ortam
// değişkenlerinden yükler. Minimalizm gereği harici config kütüphanesi
// kullanılmaz; yalnızca standart kütüphane.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config, uygulamanın tüm ayarlarını tutar.
type Config struct {
	// HTTP sunucusunun dinleyeceği adres (örn. ":8080").
	ListenAddr string

	// SQLite veritabanı dosyasının yolu.
	DatabasePath string

	// Bir izlemenin varsayılan kontrol aralığı.
	DefaultInterval time.Duration

	// RetentionDays, check_results kayıtlarının kaç gün saklanacağıdır.
	// Bu süreden eski kayıtlar pruning ile silinir.
	RetentionDays int

	// PruneInterval, pruning görevinin çalışma sıklığıdır.
	PruneInterval time.Duration

	// Workers, scheduler'daki eşzamanlı kontrol worker sayısıdır.
	Workers int

	// CheckTimeout, tek bir kontrolün (HTTP/TCP) izin verilen azami süresidir.
	CheckTimeout time.Duration

	// DispatchInterval, dispatcher'ın sırası gelmiş monitörleri taradığı
	// aralıktır. Monitör interval'larından küçük tutulmalıdır.
	DispatchInterval time.Duration

	// SessionTTL, bir oturumun geçerlilik süresidir.
	SessionTTL time.Duration

	// CookieSecure, oturum cookie'sine Secure bayrağı eklenip eklenmeyeceğidir.
	// HTTPS arkasında true yapılmalıdır.
	CookieSecure bool
}

// Load, ortam değişkenlerinden konfigürasyonu okur ve makul
// varsayılanlarla doldurur.
func Load() Config {
	return Config{
		ListenAddr:       getEnv("GOPULSE_LISTEN_ADDR", ":8080"),
		DatabasePath:     getEnv("GOPULSE_DB_PATH", "data/gopulse.db"),
		DefaultInterval:  getEnvDuration("GOPULSE_DEFAULT_INTERVAL", 60*time.Second),
		RetentionDays:    getEnvInt("GOPULSE_RETENTION_DAYS", 30),
		PruneInterval:    getEnvDuration("GOPULSE_PRUNE_INTERVAL", 24*time.Hour),
		Workers:          getEnvInt("GOPULSE_WORKERS", 10),
		CheckTimeout:     getEnvDuration("GOPULSE_CHECK_TIMEOUT", 10*time.Second),
		DispatchInterval: getEnvDuration("GOPULSE_DISPATCH_INTERVAL", 5*time.Second),
		SessionTTL:       getEnvDuration("GOPULSE_SESSION_TTL", 7*24*time.Hour),
		CookieSecure:     getEnvBool("GOPULSE_COOKIE_SECURE", false),
	}
}

// getEnv, verilen anahtar tanımlı değilse fallback değerini döndürür.
func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// getEnvInt, tam sayı biçimindeki bir ortam değişkenini ayrıştırır.
// Geçersiz veya tanımsızsa fallback döner.
func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// getEnvBool, boolean biçimindeki bir ortam değişkenini ayrıştırır
// (1/t/true/yes → true). Geçersiz veya tanımsızsa fallback döner.
func getEnvBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "t", "true", "yes", "y", "on":
			return true
		case "0", "f", "false", "no", "n", "off":
			return false
		}
	}
	return fallback
}

// getEnvDuration, süre biçimindeki bir ortam değişkenini ayrıştırır.
// Geçersiz veya tanımsızsa fallback döner.
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		// Salt sayı verilmişse saniye kabul et.
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	return fallback
}
