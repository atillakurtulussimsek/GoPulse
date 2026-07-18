package auth

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/database"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	store, err := database.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("DB açılamadı: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return New(store, time.Hour)
}

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("gizliParola1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "gizliParola1" {
		t.Fatal("şifre düz metin olarak saklanmamalı")
	}
	if !CheckPassword(hash, "gizliParola1") {
		t.Fatal("doğru şifre eşleşmeliydi")
	}
	if CheckPassword(hash, "yanlis") {
		t.Fatal("yanlış şifre eşleşmemeliydi")
	}
}

func TestSetupThenLoginFlow(t *testing.T) {
	m := newTestManager(t)

	// Başlangıçta kullanıcı yok.
	if has, _ := m.HasUsers(); has {
		t.Fatal("başlangıçta kullanıcı olmamalı")
	}

	// İlk kurulum.
	token, exp, err := m.Setup("Admin@Ornek.COM", "parola12345")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if token == "" || exp.Before(time.Now()) {
		t.Fatal("geçerli token ve gelecekteki süre bekleniyordu")
	}

	// Token geçerli kullanıcıyı çözmeli (e-posta normalize edilmiş olmalı).
	u, ok, err := m.Authenticate(token)
	if err != nil || !ok {
		t.Fatalf("oturum doğrulanmalı (ok=%v, err=%v)", ok, err)
	}
	if u.Email != "admin@ornek.com" {
		t.Fatalf("e-posta normalize edilmeliydi, gelen %q", u.Email)
	}

	// İkinci kurulum reddedilmeli.
	if _, _, err := m.Setup("baska@ornek.com", "parola12345"); !errors.Is(err, ErrSetupClosed) {
		t.Fatalf("ikinci Setup ErrSetupClosed dönmeliydi, gelen %v", err)
	}

	// Yanlış şifreyle giriş.
	if _, _, err := m.Login("admin@ornek.com", "yanlis"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("yanlış giriş ErrInvalidCredentials dönmeliydi, gelen %v", err)
	}

	// Doğru giriş (e-posta büyük/küçük harf duyarsız).
	token2, _, err := m.Login("ADMIN@ornek.com", "parola12345")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Logout sonrası token geçersiz olmalı.
	if err := m.Logout(token2); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, ok, _ := m.Authenticate(token2); ok {
		t.Fatal("logout sonrası oturum geçersiz olmalı")
	}
}

func TestSetupValidation(t *testing.T) {
	m := newTestManager(t)
	if _, _, err := m.Setup("gecersiz-eposta", "parola12345"); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("ErrInvalidEmail bekleniyordu, gelen %v", err)
	}
	if _, _, err := m.Setup("a@b.com", "kisa"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("ErrWeakPassword bekleniyordu, gelen %v", err)
	}
}
