# GoPulse

Minimalist, açık kaynaklı bir uptime izleme botu. Go ile yazılmış; **tek
binary**'ye derlenir, dosya tabanlı **SQLite** kullanır ve web arayüzü
binary'ye **gömülüdür** (harici dosya/CDN bağımlılığı yoktur; yalnızca HTMX
CDN'den yüklenir).

## Özellikler

- ✅ Modüler izleme çekirdeği (`Checker` interface): **HTTP/HTTPS** ve **TCP port** kontrolü
- ✅ Periyodik izleme (worker pool + dispatcher scheduler)
- ✅ Gömülü web paneli (Go `html/template` + HTMX + Tailwind CSS)
- ✅ Dosya tabanlı SQLite + otomatik log budama (pruning)
- ✅ Çok kullanıcılı panel koruması (bcrypt + oturum) — Multi-User, Single-Tenant
- ✅ Durum değişiminde bildirim: **Telegram** ve **SMTP (e-posta)** — panelden dinamik kanal yönetimi ve monitör-kanal eşleme
- 🔲 Ek checker türleri (SSL, Ping) — modüler yapı sayesinde kolayca eklenebilir

## Teknoloji

| Katman | Seçim |
|--------|-------|
| Dil | Go 1.23 |
| HTTP | `net/http` (standart kütüphane) |
| Veritabanı | SQLite (`modernc.org/sqlite`, CGO'suz) |
| Frontend | `html/template` + HTMX + Tailwind CSS (gömülü) |
| Kimlik doğrulama | bcrypt + DB tabanlı oturum |

## Hızlı Başlangıç

İlk çalıştırmada panele girdiğinde seni bir **kurulum ekranı** karşılar ve ilk
yönetici hesabını oluşturursun.

### Kaynaktan

```bash
git clone https://github.com/atillakurtulussimsek/GoPulse.git
cd GoPulse
make run          # http://localhost:8080
```

Tek binary derlemek için:

```bash
make build        # ./bin/gopulse
```

### Docker

```bash
make docker                                   # gopulse:latest imajını derler
docker run -d --name gopulse \
  -p 8080:8080 \
  -v gopulse-data:/data \
  gopulse:latest
```

Veri (SQLite) `/data` hacminde kalıcıdır. İmaj `distroless/static` tabanlıdır
(kabuk yok, nonroot kullanıcı, minimum saldırı yüzeyi).

### systemd

Örnek servis birimi ve kurulum adımları için
[`deploy/gopulse.service`](deploy/gopulse.service) dosyasına bakın.

## Yapılandırma (ortam değişkenleri)

Tüm ayarların makul varsayılanları vardır; hiçbirini ayarlamadan çalışır.

| Değişken | Varsayılan | Açıklama |
|----------|-----------|----------|
| `GOPULSE_LISTEN_ADDR` | `:8080` | HTTP dinleme adresi |
| `GOPULSE_DB_PATH` | `data/gopulse.db` | SQLite dosya yolu |
| `GOPULSE_DEFAULT_INTERVAL` | `60s` | Varsayılan kontrol aralığı |
| `GOPULSE_RETENTION_DAYS` | `30` | Kontrol geçmişinin saklanma süresi (gün) |
| `GOPULSE_PRUNE_INTERVAL` | `24h` | Budama görevinin çalışma sıklığı |
| `GOPULSE_WORKERS` | `10` | Eşzamanlı kontrol worker sayısı |
| `GOPULSE_CHECK_TIMEOUT` | `10s` | Tek bir kontrolün azami süresi |
| `GOPULSE_DISPATCH_INTERVAL` | `5s` | Dispatcher tarama aralığı |
| `GOPULSE_SESSION_TTL` | `168h` | Oturum geçerlilik süresi |
| `GOPULSE_COOKIE_SECURE` | `false` | HTTPS arkasında `true` yapın |

> **Üretim notu:** GoPulse'u bir HTTPS reverse proxy (nginx, Caddy vb.)
> arkasında çalıştırıp `GOPULSE_COOKIE_SECURE=true` ayarlayın.

## Geliştirme

```bash
make run          # çalıştır
make test         # testler
make vet          # statik analiz
make css          # Tailwind CSS'i yeniden derle (bkz. aşağıda)
make build        # tek binary
make docker       # Docker imajı
```

### Frontend (Tailwind)

Arayüz stilleri `internal/web/tailwind/input.css`'ten derlenip
`internal/web/static/app.css`'e yazılır ve binary'ye gömülür. Derleyici olarak
[Tailwind standalone CLI](https://github.com/tailwindlabs/tailwindcss/releases)
kullanılır (Node gerektirmez):

```bash
make css          # veya: make css TAILWIND="npx @tailwindcss/cli"
```

## Proje Yapısı

```
cmd/gopulse/        Giriş noktası (composition root)
internal/
  config/           Ortam değişkeni tabanlı yapılandırma
  models/           Ortak veri yapıları
  checker/          İzleme çekirdeği (Checker interface + HTTP/TCP)
  notifier/         Bildirim çekirdeği (Notifier interface + Telegram/SMTP)
  scheduler/        Periyodik izleme (worker pool + dispatcher)
  auth/             Kimlik doğrulama (bcrypt + oturum)
  database/         SQLite: şema, migration, pruning, sorgular
  web/              HTTP sunucu, handler'lar, gömülü template + static
deploy/             Dağıtım dosyaları (systemd)
```

## Lisans

[MIT](LICENSE) © 2026 Atilla ŞİMŞEK
