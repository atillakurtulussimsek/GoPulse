// Package web, GoPulse'un HTTP sunucusunu, handler'larını ve gömülü
// frontend varlıklarını (template + static) barındırır.
package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/auth"
	"github.com/atillakurtulussimsek/GoPulse/internal/config"
	"github.com/atillakurtulussimsek/GoPulse/internal/database"
	"github.com/atillakurtulussimsek/GoPulse/internal/models"
	"github.com/atillakurtulussimsek/GoPulse/internal/notifier"
)

// Server, HTTP katmanını ve bağımlılıklarını tutar.
type Server struct {
	cfg        config.Config
	store      *database.Store
	auth       *auth.Manager
	dispatcher *notifier.Dispatcher
	templates  *template.Template
	mux        *http.ServeMux
}

// authPageData, login/setup şablonlarına aktarılan görünüm modelidir.
type authPageData struct {
	Error string
	Email string
}

// monitorChannelsData, bir monitörün kanal eşleme formuna aktarılan modeldir.
type monitorChannelsData struct {
	MonitorID int64
	Channels  []channelChecked
}

// channelChecked, eşleme formunda bir kanalı ve seçili olup olmadığını taşır.
type channelChecked struct {
	Channel models.NotificationChannel
	Checked bool
}

// dashboardData, panel şablonlarına aktarılan görünüm modelidir.
type dashboardData struct {
	Monitors []database.MonitorStatus
	Channels []models.NotificationChannel
}

// NewServer, gömülü şablonları ayrıştırır ve rotaları kurar.
func NewServer(cfg config.Config, store *database.Store, dispatcher *notifier.Dispatcher) (*Server, error) {
	tmpl, err := template.New("").Funcs(templateFuncs()).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg:        cfg,
		store:      store,
		auth:       auth.New(store, cfg.SessionTTL),
		dispatcher: dispatcher,
		templates:  tmpl,
		mux:        http.NewServeMux(),
	}
	s.routes()
	return s, nil
}

// Handler, sunucunun http.Handler'ını döndürür.
func (s *Server) Handler() http.Handler { return s.mux }

// routes, tüm HTTP rotalarını kaydeder (Go 1.22+ pattern söz dizimi).
func (s *Server) routes() {
	// Gömülü statik dosyalar: /static/...
	staticSub, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	s.mux.HandleFunc("GET /healthz", s.handleHealthz)

	// Kimlik doğrulama uçları (korumasız).
	s.mux.HandleFunc("GET /setup", s.handleSetupForm)
	s.mux.HandleFunc("POST /setup", s.handleSetup)
	s.mux.HandleFunc("GET /login", s.handleLoginForm)
	s.mux.HandleFunc("POST /login", s.handleLogin)
	s.mux.HandleFunc("POST /logout", s.handleLogout)

	// Korumalı rotalar (oturum zorunlu).
	s.mux.HandleFunc("GET /{$}", s.requireAuth(s.handleIndex))
	s.mux.HandleFunc("GET /partials/monitors", s.requireAuth(s.handleMonitorTable))
	s.mux.HandleFunc("POST /monitors", s.requireAuth(s.handleCreateMonitor))
	s.mux.HandleFunc("DELETE /monitors/{id}", s.requireAuth(s.handleDeleteMonitor))
	s.mux.HandleFunc("POST /monitors/{id}/toggle", s.requireAuth(s.handleToggleMonitor))

	// Monitör-kanal eşleme (bildirim yönlendirme).
	s.mux.HandleFunc("GET /monitors/{id}/channels", s.requireAuth(s.handleMonitorChannelsForm))
	s.mux.HandleFunc("POST /monitors/{id}/channels", s.requireAuth(s.handleSaveMonitorChannels))

	// Bildirim kanalları yönetimi.
	s.mux.HandleFunc("GET /partials/channels", s.requireAuth(s.handleChannelTable))
	s.mux.HandleFunc("GET /channels/fields", s.requireAuth(s.handleChannelFields))
	s.mux.HandleFunc("POST /channels", s.requireAuth(s.handleCreateChannel))
	s.mux.HandleFunc("DELETE /channels/{id}", s.requireAuth(s.handleDeleteChannel))
	s.mux.HandleFunc("POST /channels/{id}/toggle", s.requireAuth(s.handleToggleChannel))
	s.mux.HandleFunc("POST /channels/{id}/test", s.requireAuth(s.handleTestChannel))
}

// requireAuth, korumalı bir handler'ı oturum kontrolüyle sarar. Hiç kullanıcı
// yoksa /setup'a, oturum yoksa /login'e yönlendirir.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		has, err := s.auth.HasUsers()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !has {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		if _, ok, err := s.auth.Authenticate(s.sessionToken(r)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// handleIndex, ana panel sayfasını render eder.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := s.dashboardData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.templates.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleMonitorTable, yalnızca monitör tablosunu (HTMX partial) render eder.
// Canlı yenileme (polling) ve CRUD sonrası güncelleme için kullanılır.
func (s *Server) handleMonitorTable(w http.ResponseWriter, r *http.Request) {
	s.renderMonitorTable(w)
}

// handleCreateMonitor, formdan yeni bir monitör oluşturur ve güncel tabloyu
// döndürür.
func (s *Server) handleCreateMonitor(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form okunamadı", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	target := strings.TrimSpace(r.FormValue("target"))
	typ := models.MonitorType(strings.TrimSpace(r.FormValue("type")))

	if name == "" || target == "" {
		http.Error(w, "ad ve hedef zorunludur", http.StatusBadRequest)
		return
	}
	if typ != models.MonitorHTTP && typ != models.MonitorTCP {
		http.Error(w, "geçersiz izleme türü", http.StatusBadRequest)
		return
	}

	interval := s.cfg.DefaultInterval
	if v := strings.TrimSpace(r.FormValue("interval")); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			interval = time.Duration(secs) * time.Second
		}
	}

	if _, err := s.store.CreateMonitor(models.Monitor{
		Name:     name,
		Type:     typ,
		Target:   target,
		Interval: interval,
		Enabled:  true,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.renderMonitorTable(w)
}

// handleDeleteMonitor, bir monitörü siler ve güncel tabloyu döndürür.
func (s *Server) handleDeleteMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "geçersiz id", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteMonitor(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderMonitorTable(w)
}

// handleToggleMonitor, bir monitörün aktif/pasif durumunu tersine çevirir ve
// güncel tabloyu döndürür.
func (s *Server) handleToggleMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "geçersiz id", http.StatusBadRequest)
		return
	}

	// Mevcut durumu bul ve tersine çevir.
	monitors, err := s.store.ListMonitors()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var found bool
	for _, m := range monitors {
		if m.ID == id {
			if err := s.store.SetMonitorEnabled(id, !m.Enabled); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "monitör bulunamadı", http.StatusNotFound)
		return
	}
	s.renderMonitorTable(w)
}

// handleSetupForm, ilk kurulum ekranını gösterir. Zaten kullanıcı varsa
// /login'e yönlendirir.
func (s *Server) handleSetupForm(w http.ResponseWriter, r *http.Request) {
	if has, err := s.auth.HasUsers(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if has {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.renderAuthPage(w, "setup", authPageData{}, http.StatusOK)
}

// handleSetup, ilk yönetici kullanıcıyı oluşturur ve oturum açar.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form okunamadı", http.StatusBadRequest)
		return
	}
	email := r.FormValue("email")
	password := r.FormValue("password")

	token, expires, err := s.auth.Setup(email, password)
	if err != nil {
		s.renderAuthPage(w, "setup", authPageData{Error: err.Error(), Email: email}, http.StatusBadRequest)
		return
	}
	s.setSessionCookie(w, token, expires)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLoginForm, giriş ekranını gösterir. Zaten oturum açıksa panele
// yönlendirir; hiç kullanıcı yoksa /setup'a.
func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	has, err := s.auth.HasUsers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !has {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if _, ok, _ := s.auth.Authenticate(s.sessionToken(r)); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.renderAuthPage(w, "login", authPageData{}, http.StatusOK)
}

// handleLogin, kimlik bilgilerini doğrular ve oturum açar.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form okunamadı", http.StatusBadRequest)
		return
	}
	email := r.FormValue("email")
	password := r.FormValue("password")

	token, expires, err := s.auth.Login(email, password)
	if err != nil {
		s.renderAuthPage(w, "login", authPageData{Error: err.Error(), Email: email}, http.StatusUnauthorized)
		return
	}
	s.setSessionCookie(w, token, expires)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLogout, oturumu sonlandırır ve cookie'yi temizler.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if err := s.auth.Logout(s.sessionToken(r)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// renderAuthPage, login/setup tam sayfasını render eder.
func (s *Server) renderAuthPage(w http.ResponseWriter, name string, data authPageData, status int) {
	w.WriteHeader(status)
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// sessionToken, istekteki oturum cookie'sinin değerini döndürür (yoksa boş).
func (s *Server) sessionToken(r *http.Request) string {
	c, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// setSessionCookie, oturum cookie'sini ayarlar.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookie, oturum cookie'sini siler.
func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// handleHealthz, basit bir sağlık kontrolü ucudur.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

// dashboardData, panel için güncel monitör durum listesini ve kanalları toplar.
func (s *Server) dashboardData() (dashboardData, error) {
	monitors, err := s.store.ListMonitorsWithStatus(time.Now().UTC().Add(-24 * time.Hour))
	if err != nil {
		return dashboardData{}, err
	}
	channels, err := s.store.ListChannels()
	if err != nil {
		return dashboardData{}, err
	}
	return dashboardData{Monitors: monitors, Channels: channels}, nil
}

// renderChannelTable, bildirim kanalı tablosu partial'ını render eder.
func (s *Server) renderChannelTable(w http.ResponseWriter) {
	channels, err := s.store.ListChannels()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.templates.ExecuteTemplate(w, "channel_table", dashboardData{Channels: channels}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleChannelTable, kanal tablosunu (HTMX partial) render eder.
func (s *Server) handleChannelTable(w http.ResponseWriter, r *http.Request) {
	s.renderChannelTable(w)
}

// handleChannelFields, seçilen kanal türüne göre yapılandırma alanlarını
// (HTMX partial) döndürür.
func (s *Server) handleChannelFields(w http.ResponseWriter, r *http.Request) {
	typ := r.URL.Query().Get("type")
	name := "channel_fields_telegram"
	if typ == notifier.TypeSMTP {
		name = "channel_fields_smtp"
	}
	if err := s.templates.ExecuteTemplate(w, name, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleCreateChannel, formdan yeni bir bildirim kanalı oluşturur.
func (s *Server) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form okunamadı", http.StatusBadRequest)
		return
	}

	label := strings.TrimSpace(r.FormValue("label"))
	typ := strings.TrimSpace(r.FormValue("type"))
	if label == "" {
		http.Error(w, "kanal etiketi zorunludur", http.StatusBadRequest)
		return
	}

	configJSON, err := buildChannelConfig(typ, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if _, err := s.store.CreateChannel(models.NotificationChannel{
		Type:    typ,
		Label:   label,
		Config:  configJSON,
		Enabled: true,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderChannelTable(w)
}

// handleDeleteChannel, bir kanalı siler.
func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "geçersiz id", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteChannel(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderChannelTable(w)
}

// handleToggleChannel, bir kanalın aktif/pasif durumunu tersine çevirir.
func (s *Server) handleToggleChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "geçersiz id", http.StatusBadRequest)
		return
	}
	ch, ok, err := s.store.GetChannel(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "kanal bulunamadı", http.StatusNotFound)
		return
	}
	if err := s.store.SetChannelEnabled(id, !ch.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderChannelTable(w)
}

// handleTestChannel, bir kanala test bildirimi gönderir ve sonucu döndürür.
func (s *Server) handleTestChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "geçersiz id", http.StatusBadRequest)
		return
	}
	ch, ok, err := s.store.GetChannel(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "kanal bulunamadı", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.dispatcher.SendTest(ch); err != nil {
		_, _ = fmt.Fprintf(w, `<span class="text-rose-400">Test başarısız: %s</span>`, template.HTMLEscapeString(err.Error()))
		return
	}
	_, _ = w.Write([]byte(`<span class="text-emerald-400">Test bildirimi gönderildi ✅</span>`))
}

// handleMonitorChannelsForm, bir monitörün kanal eşleme formunu döndürür.
func (s *Server) handleMonitorChannelsForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "geçersiz id", http.StatusBadRequest)
		return
	}

	channels, err := s.store.ListChannels()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	mappedIDs, err := s.store.ListChannelIDsForMonitor(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	mapped := make(map[int64]bool, len(mappedIDs))
	for _, cid := range mappedIDs {
		mapped[cid] = true
	}

	data := monitorChannelsData{MonitorID: id}
	for _, ch := range channels {
		data.Channels = append(data.Channels, channelChecked{Channel: ch, Checked: mapped[ch.ID]})
	}
	if err := s.templates.ExecuteTemplate(w, "monitor_channels_form", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleSaveMonitorChannels, bir monitörün kanal eşlemesini kaydeder ve
// güncel monitör tablosunu (+ modalı kapatan OOB parçayı) döndürür.
func (s *Server) handleSaveMonitorChannels(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "geçersiz id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form okunamadı", http.StatusBadRequest)
		return
	}

	var channelIDs []int64
	for _, v := range r.Form["channel"] {
		if cid, err := strconv.ParseInt(v, 10, 64); err == nil {
			channelIDs = append(channelIDs, cid)
		}
	}

	if err := s.store.SetMonitorChannels(id, channelIDs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Güncel tabloyu render et; ardından modalı kapatan OOB parçayı ekle.
	s.renderMonitorTable(w)
	_, _ = w.Write([]byte(`<div id="modal" hx-swap-oob="true"></div>`))
}

// buildChannelConfig, kanal türüne göre form alanlarını JSON yapılandırmaya
// çevirir.
func buildChannelConfig(typ string, r *http.Request) (string, error) {
	switch typ {
	case notifier.TypeTelegram:
		cfg := map[string]string{
			"token":   strings.TrimSpace(r.FormValue("token")),
			"chat_id": strings.TrimSpace(r.FormValue("chat_id")),
		}
		if cfg["token"] == "" || cfg["chat_id"] == "" {
			return "", fmt.Errorf("telegram için token ve chat_id zorunludur")
		}
		return marshalJSON(cfg)

	case notifier.TypeSMTP:
		port, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("port")))
		var to []string
		for _, addr := range strings.Split(r.FormValue("to"), ",") {
			if a := strings.TrimSpace(addr); a != "" {
				to = append(to, a)
			}
		}
		if strings.TrimSpace(r.FormValue("host")) == "" || port == 0 ||
			strings.TrimSpace(r.FormValue("from")) == "" || len(to) == 0 {
			return "", fmt.Errorf("smtp için host, port, from ve en az bir alıcı zorunludur")
		}
		cfg := map[string]any{
			"host":     strings.TrimSpace(r.FormValue("host")),
			"port":     port,
			"username": strings.TrimSpace(r.FormValue("username")),
			"password": r.FormValue("password"),
			"from":     strings.TrimSpace(r.FormValue("from")),
			"to":       to,
		}
		return marshalJSON(cfg)

	default:
		return "", fmt.Errorf("geçersiz kanal türü: %q", typ)
	}
}

// marshalJSON, bir değeri JSON metnine çevirir.
func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// renderMonitorTable, monitör tablosu partial'ını render eder.
func (s *Server) renderMonitorTable(w http.ResponseWriter) {
	data, err := s.dashboardData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.templates.ExecuteTemplate(w, "monitor_table", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// templateFuncs, şablonlarda kullanılan yardımcı fonksiyonları döndürür.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		// statusLabel, durumu Türkçe etikete çevirir.
		"statusLabel": func(s models.Status) string {
			switch s {
			case models.StatusUp:
				return "Çalışıyor"
			case models.StatusDown:
				return "Erişilemiyor"
			default:
				return "Bekliyor"
			}
		},
		// statusClasses, duruma göre rozet Tailwind sınıflarını döndürür.
		"statusClasses": func(s models.Status) string {
			switch s {
			case models.StatusUp:
				return "bg-emerald-500/15 text-emerald-400 ring-emerald-500/30"
			case models.StatusDown:
				return "bg-rose-500/15 text-rose-400 ring-rose-500/30"
			default:
				return "bg-slate-500/15 text-slate-400 ring-slate-500/30"
			}
		},
		// latencyText, gecikmeyi okunur biçime çevirir.
		"latencyText": func(d time.Duration) string {
			if d <= 0 {
				return "—"
			}
			return d.Round(time.Millisecond).String()
		},
		// timeText, bir zamanı kısa biçimde gösterir (boşsa "—").
		"timeText": func(t time.Time) string {
			if t.IsZero() {
				return "—"
			}
			return t.Local().Format("15:04:05")
		},
		// uptimeText, uptime yüzdesini biçimlendirir.
		"uptimeText": func(total int, pct float64) string {
			if total == 0 {
				return "—"
			}
			return fmt.Sprintf("%%%.1f", pct)
		},
		// intervalText, kontrol aralığını saniye olarak gösterir.
		"intervalText": func(d time.Duration) string {
			return fmt.Sprintf("%ds", int(d.Seconds()))
		},
		// channelTypeLabel, kanal türünü okunur etikete çevirir.
		"channelTypeLabel": func(t string) string {
			switch t {
			case notifier.TypeTelegram:
				return "Telegram"
			case notifier.TypeSMTP:
				return "E-posta (SMTP)"
			default:
				return t
			}
		},
	}
}
