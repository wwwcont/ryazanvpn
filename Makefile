SHELL := /bin/bash

MIGRATE ?= migrate
POSTGRES_URL ?= postgres://vpn:vpn@localhost:5432/vpn?sslmode=disable
POSTGRES_DSN ?= $(POSTGRES_URL)

.PHONY: run-single run-backend run-node test lint migrate-up migrate-down

run-single:
	docker compose --env-file .env.single.generated -f docker-compose.yml up --build

run-backend:
	docker compose --env-file .env.backend.generated -f docker-compose.backend.yml up --build

run-node:
	docker compose --env-file .env.node.generated -f docker-compose.node.yml up --build

test:
	go test ./...

lint:
	golangci-lint run

migrate-up:
	$(MIGRATE) -path migrations -database "$(POSTGRES_DSN)" up

migrate-down:
	$(MIGRATE) -path migrations -database "$(POSTGRES_DSN)" down 1
