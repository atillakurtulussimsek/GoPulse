# GoPulse

Minimalist, açık kaynaklı bir uptime izleme botu. Go ile yazılmış; tek
binary'ye derlenir, dosya tabanlı SQLite kullanır ve web arayüzü binary'ye
gömülüdür.

## Özellikler (yol haritası)

- ✅ Modüler izleme çekirdeği (`Checker` interface): HTTP/HTTPS ve TCP port kontrolü
- ✅ Gömülü web paneli (Go `html/template` + HTMX + Tailwind CSS)
- 🔲 Dosya tabanlı SQLite + otomatik log budama (pruning)
- 🔲 Periyodik izleme zamanlayıcısı (scheduler)
- 🔲 Çoklu alıcılı bildirimler (Telegram, SMTP) — panelden dinamik yönetim
- 🔲 Çok kullanıcılı tek-organizasyon paneli (Multi-User, Single-Tenant)

## Teknoloji

| Katman | Seçim |
|--------|-------|
| Dil | Go 1.23 |
| HTTP | `net/http` (standart kütüphane) |
| Veritabanı | SQLite (`modernc.org/sqlite`, CGO'suz) |
| Frontend | `html/template` + HTMX + Tailwind CSS (gömülü) |

## Geliştirme

```bash
# Bağımlılıkları düzenle
make tidy

# Çalıştır
make run

# Binary derle
make build
```

Sunucu varsayılan olarak `:8080` adresinde çalışır
(`GOPULSE_LISTEN_ADDR` ile değiştirilebilir).

## Lisans

Açık kaynak. (Lisans dosyası eklenecek.)
