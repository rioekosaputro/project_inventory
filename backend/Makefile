.PHONY: dev build run test test-integration lint tidy fmt \
        migrate-up migrate-down migrate-new seed docker-build

GO ?= go
BIN := bin

dev:            ## jalankan API (membaca backend/.env)
	$(GO) run ./cmd/api

build:
	$(GO) build -o $(BIN)/api ./cmd/api

run: build
	./$(BIN)/api

test:           ## unit test
	$(GO) test -race ./...

test-integration: ## test integrasi; butuh TEST_DATABASE_URL (Postgres lokal) — tanpa itu test di-skip
	$(GO) test -race -count=1 -tags=integration ./...

lint:
	$(GO) vet ./...
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint belum terpasang, hanya go vet yang dijalankan"

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

# --- Target di bawah berfungsi setelah T0.3 (cmd/migrate) dan T0.4 (cmd/seed) ---
migrate-up:
	$(GO) run ./cmd/migrate up

migrate-down:
	$(GO) run ./cmd/migrate down 1

migrate-new:    ## make migrate-new name=add_xxx
	@test -n "$(name)" || (echo "usage: make migrate-new name=xxx" && exit 1)
	$(GO) run ./cmd/migrate new $(name)

seed:
	$(GO) run ./cmd/seed

docker-build:
	docker build -t rental-inventory-api:dev .
