// Package web, GoPulse'un HTTP sunucusunu, handler'larını ve gömülü
// frontend varlıklarını (template + static) barındırır.
package web

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/config"
	"github.com/atillakurtulussimsek/GoPulse/internal/database"
	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// Server, HTTP katmanını ve bağımlılıklarını tutar.
type Server struct {
	cfg       config.Config
	store     *database.Store
	templates *template.Template
	mux       *http.ServeMux
}

// dashboardData, panel şablonlarına aktarılan görünüm modelidir.
type dashboardData struct {
	Monitors []database.MonitorStatus
}

// NewServer, gömülü şablonları ayrıştırır ve rotaları kurar.
func NewServer(cfg config.Config, store *database.Store) (*Server, error) {
	tmpl, err := template.New("").Funcs(templateFuncs()).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg:       cfg,
		store:     store,
		templates: tmpl,
		mux:       http.NewServeMux(),
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
	s.mux.HandleFunc("GET /{$}", s.handleIndex)

	// Monitör CRUD (HTMX). Yanıtlar güncel tabloyu (partial) döndürür.
	s.mux.HandleFunc("GET /partials/monitors", s.handleMonitorTable)
	s.mux.HandleFunc("POST /monitors", s.handleCreateMonitor)
	s.mux.HandleFunc("DELETE /monitors/{id}", s.handleDeleteMonitor)
	s.mux.HandleFunc("POST /monitors/{id}/toggle", s.handleToggleMonitor)
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

// handleHealthz, basit bir sağlık kontrolü ucudur.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

// dashboardData, panel için güncel monitör durum listesini toplar.
func (s *Server) dashboardData() (dashboardData, error) {
	monitors, err := s.store.ListMonitorsWithStatus(time.Now().UTC().Add(-24 * time.Hour))
	if err != nil {
		return dashboardData{}, err
	}
	return dashboardData{Monitors: monitors}, nil
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
	}
}
