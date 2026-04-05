ROOT := ../..
SERVICE_DIR := services/control-plane
ENV_FILE ?= $(SERVICE_DIR)/.env.generated
COMPOSE_FILE := $(SERVICE_DIR)/docker-compose.yml

.PHONY: build test deploy up down

build:
	cd $(ROOT) && go build ./cmd/control-plane

test:
	cd $(ROOT) && go test ./cmd/control-plane ./internal/transport/httpcontrol ./internal/app

deploy:
	cd $(ROOT) && ./scripts/compose-with-env.sh $(ENV_FILE) -f $(COMPOSE_FILE) up -d --build

up: deploy

down:
	cd $(ROOT) && ./scripts/compose-with-env.sh $(ENV_FILE) -f $(COMPOSE_FILE) down
