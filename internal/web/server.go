// Package web, GoPulse'un HTTP sunucusunu, handler'larını ve gömülü
// frontend varlıklarını (template + static) barındırır.
package web

import (
	"html/template"
	"io/fs"
	"net/http"

	"github.com/atillakurtulussimsek/GoPulse/internal/config"
)

// Server, HTTP katmanını ve bağımlılıklarını tutar.
type Server struct {
	cfg       config.Config
	templates *template.Template
	mux       *http.ServeMux
}

// NewServer, gömülü şablonları ayrıştırır ve rotaları kurar.
func NewServer(cfg config.Config) (*Server, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg:       cfg,
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
}

// handleIndex, ana panel sayfasını render eder.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Title": "GoPulse",
	}
	if err := s.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleHealthz, basit bir sağlık kontrolü ucudur.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}
