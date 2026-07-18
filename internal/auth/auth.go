// Package auth, GoPulse'un kimlik doğrulama çekirdeğidir: bcrypt ile şifre
// hash'leme ve DB tabanlı oturum (session) yönetimi. Taşımadan (HTTP)
// bağımsızdır; cookie/yönlendirme mantığı web katmanındadır.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/atillakurtulussimsek/GoPulse/internal/database"
	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// SessionCookieName, oturum token'ının taşındığı cookie adıdır.
const SessionCookieName = "gopulse_session"

// minPasswordLen, kabul edilen en kısa şifre uzunluğudur.
const minPasswordLen = 8

// Doğrulama ve iş kuralı hataları.
var (
	ErrInvalidCredentials = errors.New("e-posta veya şifre hatalı")
	ErrSetupClosed        = errors.New("kurulum zaten tamamlanmış")
	ErrInvalidEmail       = errors.New("geçerli bir e-posta adresi girin")
	ErrWeakPassword       = errors.New("şifre en az 8 karakter olmalıdır")
)

// Manager, kimlik doğrulama işlemlerini yürütür.
type Manager struct {
	store *database.Store
	ttl   time.Duration
}

// New, verilen store ve oturum süresiyle bir Manager oluşturur.
func New(store *database.Store, sessionTTL time.Duration) *Manager {
	return &Manager{store: store, ttl: sessionTTL}
}

// HashPassword, bir şifreyi bcrypt ile hash'ler.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword, düz şifrenin bcrypt hash'iyle eşleşip eşleşmediğini döndürür.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// HasUsers, sistemde en az bir kullanıcı olup olmadığını döndürür.
// İlk çalıştırma (setup) tespiti için kullanılır.
func (m *Manager) HasUsers() (bool, error) {
	n, err := m.store.CountUsers()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Setup, ilk yönetici kullanıcıyı oluşturur ve bir oturum başlatır. Sistemde
// zaten kullanıcı varsa ErrSetupClosed döner.
func (m *Manager) Setup(email, password string) (token string, expiresAt time.Time, err error) {
	has, err := m.HasUsers()
	if err != nil {
		return "", time.Time{}, err
	}
	if has {
		return "", time.Time{}, ErrSetupClosed
	}

	email = normalizeEmail(email)
	if err := validateCredentials(email, password); err != nil {
		return "", time.Time{}, err
	}

	hash, err := HashPassword(password)
	if err != nil {
		return "", time.Time{}, err
	}
	uid, err := m.store.CreateUser(email, hash)
	if err != nil {
		return "", time.Time{}, err
	}
	return m.createSession(uid)
}

// Login, kimlik bilgilerini doğrular ve başarılıysa bir oturum başlatır.
// Hatalı bilgide ErrInvalidCredentials döner.
func (m *Manager) Login(email, password string) (token string, expiresAt time.Time, err error) {
	email = normalizeEmail(email)
	u, ok, err := m.store.GetUserByEmail(email)
	if err != nil {
		return "", time.Time{}, err
	}
	if !ok || !CheckPassword(u.PasswordHash, password) {
		return "", time.Time{}, ErrInvalidCredentials
	}
	return m.createSession(u.ID)
}

// Logout, verilen oturumu sonlandırır.
func (m *Manager) Logout(token string) error {
	if token == "" {
		return nil
	}
	return m.store.DeleteSession(token)
}

// Authenticate, bir oturum token'ından geçerli kullanıcıyı çözer. Oturum
// yok/süresi dolmuşsa ikinci dönüş değeri false olur.
func (m *Manager) Authenticate(token string) (models.User, bool, error) {
	if token == "" {
		return models.User{}, false, nil
	}
	return m.store.GetSessionUser(token, time.Now().UTC())
}

// createSession, rastgele bir token üretir ve oturumu kaydeder.
func (m *Manager) createSession(userID int64) (string, time.Time, error) {
	token, err := newToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().UTC().Add(m.ttl)
	if err := m.store.CreateSession(token, userID, expiresAt); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

// newToken, kriptografik olarak güvenli, rastgele bir oturum token'ı üretir.
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// normalizeEmail, e-postayı boşluklardan arındırıp küçük harfe çevirir.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// validateCredentials, e-posta ve şifre için asgari kuralları uygular.
func validateCredentials(email, password string) error {
	if email == "" || !strings.Contains(email, "@") {
		return ErrInvalidEmail
	}
	if len(password) < minPasswordLen {
		return ErrWeakPassword
	}
	return nil
}
