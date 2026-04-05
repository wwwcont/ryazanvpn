ROOT := ../..

.PHONY: build test deploy

build:
	cd $(ROOT) && go build ./cmd/control-plane

test:
	cd $(ROOT) && go test ./cmd/control-plane ./internal/transport/httpcontrol ./internal/app

deploy:
	cd $(ROOT) && ./scripts/compose-with-env.sh .env.backend.generated -f docker-compose.backend.yml up -d --build
