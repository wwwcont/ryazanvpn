SHELL := /bin/bash

MIGRATE ?= migrate
POSTGRES_URL ?= postgres://ryazanvpn:ryazanvpn_local_pass@localhost:5432/ryazanvpn?sslmode=disable
POSTGRES_DSN ?= $(POSTGRES_URL)
SINGLE_ENV ?= .env.single.generated
SINGLE_COMPOSE = ./scripts/compose-with-env.sh $(SINGLE_ENV) -f docker-compose.single.yml

.PHONY: \
	single run-single up-single down-single rebuild-single logs-single logs-control logs-agent ps-single restart-control restart-agent \
	single-vpn-up single-runtime-sync single-control-up single-node-up \
	topology-runtime-up topology-sync-env topology-control-up topology-node-up topology-ps topology-down \
	run-backend run-node \
	test lint migrate-up migrate-down

single:
	$(SINGLE_COMPOSE) up -d --build

run-single:
	$(SINGLE_COMPOSE) up -d --build

up-single:
	$(SINGLE_COMPOSE) up -d --build

down-single:
	$(SINGLE_COMPOSE) down

rebuild-single:
	$(SINGLE_COMPOSE) up -d --build --force-recreate

logs-single:
	$(SINGLE_COMPOSE) logs -f

logs-control:
	$(SINGLE_COMPOSE) logs -f control-plane

logs-agent:
	$(SINGLE_COMPOSE) logs -f node-agent

ps-single:
	$(SINGLE_COMPOSE) ps

restart-control:
	$(SINGLE_COMPOSE) restart control-plane

restart-agent:
	$(SINGLE_COMPOSE) restart node-agent

single-vpn-up:
	$(SINGLE_COMPOSE) up -d --build amnezia-awg xray

single-runtime-sync:
	./scripts/single-runtime-sync.sh $(SINGLE_ENV)

single-control-up:
	$(SINGLE_COMPOSE) up -d --build postgres redis migrate control-plane

single-node-up:
	$(SINGLE_COMPOSE) up -d --build node-agent

topology-runtime-up:
	ENV_FILE=$(SINGLE_ENV) ./scripts/topology-flow.sh runtime-up

topology-sync-env:
	ENV_FILE=$(SINGLE_ENV) ./scripts/topology-flow.sh sync-env

topology-control-up:
	ENV_FILE=$(SINGLE_ENV) ./scripts/topology-flow.sh control-up

topology-node-up:
	ENV_FILE=$(SINGLE_ENV) ./scripts/topology-flow.sh node-up

topology-ps:
	ENV_FILE=$(SINGLE_ENV) ./scripts/topology-flow.sh ps

topology-down:
	ENV_FILE=$(SINGLE_ENV) ./scripts/topology-flow.sh down

run-backend:
	./scripts/compose-with-env.sh .env.backend.generated -f docker-compose.backend.yml up -d --build

run-node:
	./scripts/compose-with-env.sh .env.node.generated -f docker-compose.node.yml up -d --build

test:
	go test ./...

lint:
	golangci-lint run

migrate-up:
	$(MIGRATE) -path migrations -database "$(POSTGRES_DSN)" up

migrate-down:
	$(MIGRATE) -path migrations -database "$(POSTGRES_DSN)" down 1
