# syntax=docker/dockerfile:1

# ---- Derleme aşaması ----
# CGO'suz (modernc.org/sqlite saf Go) tamamen statik binary üretilir.
FROM golang:1.23-bookworm AS build

# Pinlenmiş sürümlerle deterministik derleme (toolchain indirmeyi engelle).
ENV GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux

WORKDIR /src

# Bağımlılıkları önce çek (katman önbelleği).
COPY go.mod go.sum ./
RUN go mod download

# Kaynağı kopyala ve derle. Gömülü frontend (app.css, template'ler) go:embed
# ile binary'ye dahil edilir; ek derleme adımı gerekmez.
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/gopulse ./cmd/gopulse

# Nonroot kullanıcının yazabileceği veri dizinini hazırla.
RUN mkdir -p /out/data

# ---- Çalışma aşaması ----
# distroless/static: yalnızca binary + CA sertifikaları (TLS için) + tzdata.
# Kabuk/paket yöneticisi yok → minimum saldırı yüzeyi.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/gopulse /gopulse
# distroless nonroot kullanıcısı uid/gid 65532'dir.
COPY --from=build --chown=65532:65532 /out/data /data

ENV GOPULSE_LISTEN_ADDR=":8080" \
    GOPULSE_DB_PATH="/data/gopulse.db"

EXPOSE 8080
VOLUME ["/data"]
USER nonroot

ENTRYPOINT ["/gopulse"]
