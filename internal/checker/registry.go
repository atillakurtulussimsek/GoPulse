package checker

import (
	"fmt"

	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// Registry, izleme türlerini (MonitorType) ilgili Checker
// implementasyonlarına eşler. Scheduler, bir izlemenin türüne göre
// doğru checker'ı buradan çözer.
type Registry struct {
	checkers map[models.MonitorType]Checker
}

// NewRegistry, boş bir registry oluşturur.
func NewRegistry() *Registry {
	return &Registry{checkers: make(map[models.MonitorType]Checker)}
}

// Register, bir checker'ı Type() anahtarıyla kaydeder.
func (r *Registry) Register(c Checker) {
	r.checkers[c.Type()] = c
}

// Get, verilen türe karşılık gelen checker'ı döndürür.
// Kayıtlı değilse hata döner.
func (r *Registry) Get(t models.MonitorType) (Checker, error) {
	c, ok := r.checkers[t]
	if !ok {
		return nil, fmt.Errorf("checker kayıtlı değil: %q", t)
	}
	return c, nil
}
