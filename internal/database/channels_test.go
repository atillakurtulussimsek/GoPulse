package database

import (
	"testing"
	"time"

	"github.com/atillakurtulussimsek/GoPulse/internal/models"
)

// TestChannelCRUD, kanal oluşturma/listeleme/getirme/toggle/silmeyi doğrular.
func TestChannelCRUD(t *testing.T) {
	s := openTestStore(t)

	id, err := s.CreateChannel(models.NotificationChannel{
		Type:    "telegram",
		Label:   "Ekip Kanalı",
		Config:  `{"chat_id":"123"}`,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	ch, ok, err := s.GetChannel(id)
	if err != nil || !ok {
		t.Fatalf("kanal bulunmalı (ok=%v, err=%v)", ok, err)
	}
	if ch.Type != "telegram" || ch.Label != "Ekip Kanalı" || !ch.Enabled {
		t.Fatalf("kanal alanları hatalı: %+v", ch)
	}

	if err := s.SetChannelEnabled(id, false); err != nil {
		t.Fatalf("SetChannelEnabled: %v", err)
	}
	if ch, _, _ := s.GetChannel(id); ch.Enabled {
		t.Fatal("kanal pasif olmalıydı")
	}

	channels, _ := s.ListChannels()
	if len(channels) != 1 {
		t.Fatalf("1 kanal bekleniyor, bulunan %d", len(channels))
	}

	if err := s.DeleteChannel(id); err != nil {
		t.Fatalf("DeleteChannel: %v", err)
	}
	if _, ok, _ := s.GetChannel(id); ok {
		t.Fatal("kanal silinmeliydi")
	}
}

// TestMonitorChannelMapping, monitör-kanal eşlemesini ve yalnızca aktif
// kanalların döndürülmesini doğrular.
func TestMonitorChannelMapping(t *testing.T) {
	s := openTestStore(t)

	mID, _ := s.CreateMonitor(models.Monitor{
		Name: "M", Type: models.MonitorHTTP, Target: "https://m", Interval: time.Minute, Enabled: true,
	})
	c1, _ := s.CreateChannel(models.NotificationChannel{Type: "telegram", Label: "A", Enabled: true})
	c2, _ := s.CreateChannel(models.NotificationChannel{Type: "smtp", Label: "B", Enabled: false})
	c3, _ := s.CreateChannel(models.NotificationChannel{Type: "telegram", Label: "C", Enabled: true})

	// c1 ve c2'yi eşle.
	if err := s.SetMonitorChannels(mID, []int64{c1, c2}); err != nil {
		t.Fatalf("SetMonitorChannels: %v", err)
	}
	ids, _ := s.ListChannelIDsForMonitor(mID)
	if len(ids) != 2 {
		t.Fatalf("2 eşleme bekleniyor, bulunan %d", len(ids))
	}

	// Aktif kanallar: yalnızca c1 (c2 pasif).
	active, err := s.ListActiveChannelsForMonitor(mID)
	if err != nil {
		t.Fatalf("ListActiveChannelsForMonitor: %v", err)
	}
	if len(active) != 1 || active[0].ID != c1 {
		t.Fatalf("yalnızca aktif c1 dönmeli, gelen %+v", active)
	}

	// Eşlemeyi c3 ile değiştir (c1/c2 kalkar).
	if err := s.SetMonitorChannels(mID, []int64{c3}); err != nil {
		t.Fatalf("SetMonitorChannels (değiştir): %v", err)
	}
	ids, _ = s.ListChannelIDsForMonitor(mID)
	if len(ids) != 1 || ids[0] != c3 {
		t.Fatalf("eşleme c3 olmalı, gelen %v", ids)
	}

	// Kanal silinince eşleme de kalkmalı (FK CASCADE).
	_ = s.DeleteChannel(c3)
	ids, _ = s.ListChannelIDsForMonitor(mID)
	if len(ids) != 0 {
		t.Fatalf("kanal silinince eşleme kalkmalı, kalan %v", ids)
	}
}
