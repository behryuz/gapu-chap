-include .env
export

MIGRATIONS_DIR := migrations
GOOSE := go tool goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)"

.PHONY: run build db-up db-down migrate-up migrate-down migrate-status migrate-create test lint test-integration promote up down

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test -race -count=1 ./...

db-up:
	docker compose up -d

db-down:
	docker compose down

migrate-up:
	@$(GOOSE) up

migrate-down:
	@$(GOOSE) down

migrate-status:
	@$(GOOSE) status

# использование: make migrate-create name=create_rooms
migrate-create:
	go tool goose -dir $(MIGRATIONS_DIR) -s create $(name) sql

lint:
	go tool golangci-lint run ./...
# Интеграционные тесты: каждый тест создаёт свою временную базу на сервере из docker compose.
TEST_DATABASE_URL ?= postgres://chat:chat@localhost:5434/postgres?sslmode=disable

test-integration:
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -race -count=1 ./...

# Назначить модератора: make promote username=alice
promote:
	@test -n "$(username)" || (echo "usage: make promote username=<name>" && exit 1)
	docker compose exec -T postgres psql -U chat -d chat -v ON_ERROR_STOP=1 \
		-c "UPDATE users SET role = 'moderator' WHERE lower(username) = lower('$(username)')"

# Всё приложение в Docker: PostgreSQL + бэкенд + фронтенд (из ../gapu-chap-front) на http://localhost:8080
up:
	docker compose --profile app up -d --build

# Останавливает только приложение; база для локальной разработки продолжает работать (make db-down).
down:
	docker compose --profile app rm --stop --force app web
