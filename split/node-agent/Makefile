SHELL := /bin/bash
ENV_FILE ?= .env.generated

.PHONY: build test up down logs

build:
	go build ./cmd/node-agent

test:
	go test ./cmd/node-agent ./internal/transport/httpnode ./internal/agent/...

up:
	docker compose --env-file $(ENV_FILE) -f docker-compose.yml up -d --build

down:
	docker compose --env-file $(ENV_FILE) -f docker-compose.yml down

logs:
	docker compose --env-file $(ENV_FILE) -f docker-compose.yml logs -f
