.PHONY: help run build tidy fmt vet lint test test-race-docker migrate-up migrate-down migrate-create seed-admin

help:
	@echo "Rajaku Printing backend — available targets:"
	@echo "  run              Run API server (loads backend/.env)"
	@echo "  build            Build binary to ./bin/api"
	@echo "  tidy             go mod tidy"
	@echo "  fmt              gofmt + goimports"
	@echo "  vet              go vet ./..."
	@echo "  lint             golangci-lint run"
	@echo "  test             go test ./..."
	@echo "  migrate-up       Apply DB migrations"
	@echo "  migrate-down     Rollback last migration"
	@echo "  migrate-create name=xxx  Create new migration files"
	@echo "  seed-admin email=x phone=y name=z password=p  Create super admin (idempotent)"

run:
	go run ./cmd/api

build:
	go build -o bin/api ./cmd/api

tidy:
	go mod tidy

fmt:
	gofmt -w .
	@command -v goimports >/dev/null 2>&1 && goimports -w . || echo "goimports not installed"

vet:
	go vet ./...

lint:
	golangci-lint run

test:
	go test -race -cover ./...

# Race detector butuh cgo + compiler C. Mesin dev Windows di sini tidak punya
# gcc, jadi `make test` gagal di `-race`. Target ini menjalankan suite yang
# sama di dalam container Linux (toolchain C lengkap) — hasilnya identik
# dengan job `go test -race` di .github/workflows/ci.yml.
# Volume cache dipakai ulang supaya run kedua dst. tidak download modul lagi.
test-race-docker:
	docker run --rm \
	  -v "$(CURDIR):/src" \
	  -v rajaku-gomod:/go/pkg/mod \
	  -v rajaku-gocache:/root/.cache/go-build \
	  -w /src -e CGO_ENABLED=1 -e GOFLAGS=-buildvcs=false \
	  golang:1.26 go test -race -cover ./...

# --- DB migrations (golang-migrate) ---
# Requires: `go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest`
DB_URL ?= postgres://rajaku:rajaku_dev@localhost:5432/rajaku_printing?sslmode=disable

migrate-up:
	migrate -database "$(DB_URL)" -path migrations up

migrate-down:
	migrate -database "$(DB_URL)" -path migrations down 1

migrate-create:
	@test -n "$(name)" || (echo "usage: make migrate-create name=create_users_table"; exit 1)
	migrate create -ext sql -dir migrations -seq $(name)

seed-admin:
	@test -n "$(email)" && test -n "$(phone)" && test -n "$(name)" && test -n "$(password)" \
	  || (echo "usage: make seed-admin email=x phone=y name=z password=p"; exit 1)
	go run ./cmd/seedadmin --email "$(email)" --phone "$(phone)" --name "$(name)" --password "$(password)"
