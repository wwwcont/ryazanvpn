ROOT := ../..

.PHONY: build test deploy

build:
	cd $(ROOT) && go build ./cmd/node-agent

test:
	cd $(ROOT) && go test ./cmd/node-agent ./internal/transport/httpnode ./internal/agent/...

deploy:
	cd $(ROOT) && ./scripts/compose-with-env.sh .env.node.generated -f docker-compose.node.yml up -d --build
