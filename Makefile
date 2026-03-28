SHELL := /bin/bash

MIGRATE ?= migrate
POSTGRES_URL ?= postgres://vpn:vpn@localhost:5432/vpn?sslmode=disable
POSTGRES_DSN ?= $(POSTGRES_URL)

.PHONY: run test lint migrate-up migrate-down

run:
	docker compose up --build

test:
	go test ./...

lint:
	golangci-lint run

migrate-up:
	$(MIGRATE) -path migrations -database "$(POSTGRES_DSN)" up

migrate-down:
	$(MIGRATE) -path migrations -database "$(POSTGRES_DSN)" down 1
