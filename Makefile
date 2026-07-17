# GoPulse — geliştirme komutları

BINARY := gopulse
CMD    := ./cmd/gopulse

# Tailwind CSS derleyicisi. Varsayılan: PATH üzerindeki standalone CLI
# (Node gerektirmez — https://github.com/tailwindlabs/tailwindcss/releases).
# Alternatif olarak npx kullanmak için: make css TAILWIND="npx @tailwindcss/cli"
TAILWIND    := tailwindcss
CSS_INPUT   := internal/web/tailwind/input.css
CSS_OUTPUT  := internal/web/static/app.css

.PHONY: run build tidy fmt vet clean css css-watch

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

## css: Tailwind CSS'i derle (gömülen app.css'i üretir)
css:
	$(TAILWIND) -i $(CSS_INPUT) -o $(CSS_OUTPUT) --minify

## css-watch: Geliştirme sırasında CSS'i izleyerek derle
css-watch:
	$(TAILWIND) -i $(CSS_INPUT) -o $(CSS_OUTPUT) --watch

## clean: Derleme çıktısını temizle
clean:
	rm -rf bin/
