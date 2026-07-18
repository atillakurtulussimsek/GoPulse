package database

import (
	"testing"
	"time"
)

// TestUserCRUD, kullanıcı oluşturma, sayma ve e-postayla getirmeyi doğrular.
func TestUserCRUD(t *testing.T) {
	s := openTestStore(t)

	if n, err := s.CountUsers(); err != nil || n != 0 {
		t.Fatalf("başlangıçta 0 kullanıcı bekleniyor (n=%d, err=%v)", n, err)
	}

	id, err := s.CreateUser("admin@ornek.com", "hash123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if n, _ := s.CountUsers(); n != 1 {
		t.Fatalf("1 kullanıcı bekleniyor, bulunan %d", n)
	}

	u, ok, err := s.GetUserByEmail("admin@ornek.com")
	if err != nil || !ok {
		t.Fatalf("kullanıcı bulunmalı (ok=%v, err=%v)", ok, err)
	}
	if u.ID != id || u.PasswordHash != "hash123" {
		t.Fatalf("kullanıcı alanları hatalı: %+v", u)
	}

	// Olmayan kullanıcı.
	if _, ok, _ := s.GetUserByEmail("yok@ornek.com"); ok {
		t.Fatal("olmayan kullanıcı bulundu")
	}

	// E-posta benzersiz olmalı.
	if _, err := s.CreateUser("admin@ornek.com", "x"); err == nil {
		t.Fatal("tekrarlanan e-posta hata vermeliydi")
	}
}

// TestSessionLifecycle, oturum oluşturma, geçerlilik, süre dolumu, silme ve
// temizlemeyi doğrular.
func TestSessionLifecycle(t *testing.T) {
	s := openTestStore(t)
	uid, _ := s.CreateUser("u@ornek.com", "h")

	now := time.Now().UTC()

	// Geçerli oturum.
	if err := s.CreateSession("tok-valid", uid, now.Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if u, ok, err := s.GetSessionUser("tok-valid", now); err != nil || !ok || u.ID != uid {
		t.Fatalf("geçerli oturum kullanıcı döndürmeli (ok=%v, err=%v)", ok, err)
	}

	// Süresi dolmuş oturum geçersiz sayılmalı.
	_ = s.CreateSession("tok-expired", uid, now.Add(-time.Hour))
	if _, ok, _ := s.GetSessionUser("tok-expired", now); ok {
		t.Fatal("süresi dolmuş oturum geçerli sayıldı")
	}

	// Silme (logout).
	if err := s.DeleteSession("tok-valid"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, ok, _ := s.GetSessionUser("tok-valid", now); ok {
		t.Fatal("silinen oturum hâlâ geçerli")
	}

	// Süresi dolmuş oturumların temizlenmesi.
	deleted, err := s.DeleteExpiredSessions(now)
	if err != nil {
		t.Fatalf("DeleteExpiredSessions: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("1 süresi dolmuş oturum silinmeli, silinen %d", deleted)
	}
}
