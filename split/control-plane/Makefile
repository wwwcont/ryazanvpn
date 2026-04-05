SHELL := /bin/bash
ENV_FILE ?= .env

.PHONY: build test up down logs migrate

build:
	go build ./cmd/control-plane

test:
	go test ./cmd/control-plane ./internal/transport/httpcontrol ./internal/app

up:
	docker compose --env-file $(ENV_FILE) -f docker-compose.yml up -d --build

down:
	docker compose --env-file $(ENV_FILE) -f docker-compose.yml down

logs:
	docker compose --env-file $(ENV_FILE) -f docker-compose.yml logs -f

migrate:
	docker compose --env-file $(ENV_FILE) -f docker-compose.yml run --rm migrate
