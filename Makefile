-include .env
export

MIGRATIONS_DIR := migrations
GOOSE := go tool goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)"

.PHONY: run build db-up db-down migrate-up migrate-down migrate-status migrate-create

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

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