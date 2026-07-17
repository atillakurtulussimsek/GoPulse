# GoPulse — geliştirme komutları

BINARY := gopulse
CMD    := ./cmd/gopulse

.PHONY: run build tidy fmt vet clean

## run: Uygulamayı çalıştır
run:
	go run $(CMD)

## build: Tek binary derle
build:
	go build -o bin/$(BINARY) $(CMD)

## tidy: Bağımlılıkları düzenle
tidy:
	go mod tidy

## fmt: Kodu biçimlendir
fmt:
	go fmt ./...

## vet: Statik analiz
vet:
	go vet ./...

## clean: Derleme çıktısını temizle
clean:
	rm -rf bin/
