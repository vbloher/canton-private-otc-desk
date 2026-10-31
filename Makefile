LOCAL_BIN := $(CURDIR)/bin

PROTOC_VERSION = 36.2
GOLANGCI_LINT_VERSION = v2.15.0

PROTOC := $(LOCAL_BIN)/protoc/bin/protoc
PROTOC_OS := $(shell uname -s | sed 's/Darwin/osx/;s/Linux/linux/')
PROTOC_ARCH := $(shell uname -m | sed 's/arm64/aarch_64/;s/aarch64/aarch_64/')

PROTOS := identity_v1 ledger_v1

SERVICES := gateway identity trading ledger maker-bot

OBS := GRAFANA_ADMIN_PASSWORD=$${GRAFANA_ADMIN_PASSWORD:-admin} docker compose -f deploy/observability/docker-compose.yml

.PHONY: lint test integration-test vuln env network up up-all down clean obs-up obs-down web-dev generate grpc-gen \
	bin-deps protoc-install lint-install

run-%:
	@test -f services/$*/.env || cp services/$*/.env.example services/$*/.env
	cd services/$* && go run ./cmd/app

env:
	@for s in $(SERVICES); do test -f services/$$s/.env || cp services/$$s/.env.example services/$$s/.env; done

lint:
	$(LOCAL_BIN)/golangci-lint run ./...

test:
	go test -race -shuffle=on -cover ./...

integration-test:
	go test -count=1 -tags=integration ./services/...

vuln:
	go tool govulncheck ./...

network:
	@docker network inspect observability >/dev/null 2>&1 || docker network create observability >/dev/null

up:
	docker compose up -d --wait postgres redis

up-all: env network
	docker compose up -d --build --wait

down:
	docker compose down

clean:
	docker compose down -v

obs-up: network
	$(OBS) up -d --wait --force-recreate

obs-down:
	$(OBS) down

web-dev:
	pnpm --dir web dev

generate: grpc-gen
	go generate ./...

grpc-gen:
	@for p in $(PROTOS); do \
	  mkdir -p ./gen/grpc/$$p; \
	  $(PROTOC) \
	    --proto_path=api/grpc \
	    --go_out=./gen/grpc/$$p --go_opt=paths=source_relative \
	    --plugin=protoc-gen-go=$(LOCAL_BIN)/protoc-gen-go \
	    --go-grpc_out=./gen/grpc/$$p --go-grpc_opt=paths=source_relative \
	    --plugin=protoc-gen-go-grpc=$(LOCAL_BIN)/protoc-gen-go-grpc \
	    $$p.proto || exit 1; \
	done

bin-deps: protoc-install lint-install

protoc-install:
	mkdir -p $(LOCAL_BIN)
	curl -sSfL -o $(LOCAL_BIN)/protoc.zip \
	  https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/protoc-$(PROTOC_VERSION)-$(PROTOC_OS)-$(PROTOC_ARCH).zip
	rm -rf $(LOCAL_BIN)/protoc
	unzip -q $(LOCAL_BIN)/protoc.zip -d $(LOCAL_BIN)/protoc
	rm $(LOCAL_BIN)/protoc.zip
	GOBIN=$(LOCAL_BIN) go install google.golang.org/protobuf/cmd/protoc-gen-go google.golang.org/grpc/cmd/protoc-gen-go-grpc

lint-install:
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(LOCAL_BIN) $(GOLANGCI_LINT_VERSION)
