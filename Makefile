LOCAL_BIN := $(CURDIR)/bin

PROTOC_VERSION = 36.2
GOLANGCI_LINT_VERSION = v2.15.0

PROTOC := $(LOCAL_BIN)/protoc/bin/protoc
PROTOC_OS := $(shell uname -s | sed 's/Darwin/osx/;s/Linux/linux/')
PROTOC_ARCH := $(shell uname -m | sed 's/arm64/aarch_64/;s/aarch64/aarch_64/')

PROTOS := identity_v1 ledger_v1

# On Linux the app containers share the host network to reach the sandbox on loopback.
COMPOSE := docker compose -f docker-compose.yml $(if $(filter Linux,$(shell uname -s)),-f docker-compose.host.yml)

.PHONY: lint test integration-test e2e-test sandbox-test vuln ledger up up-all down clean web-dev generate grpc-gen \
	bin-deps protoc-install lint-install

# run-identity, run-ledger, run-trading, run-gateway, run-maker-bot (creates services/<name>/.env from the example)
run-%:
	@test -f services/$*/.env || cp services/$*/.env.example services/$*/.env
	cd services/$* && go run ./cmd/app

lint:
	$(LOCAL_BIN)/golangci-lint run ./...

test:
	go test -race -shuffle=on -cover ./...

# black-box suite against the running stack (make ledger + make up-all)
e2e-test:
	go test -count=1 -v -tags=e2e ./test/e2e

# ledger adapter against the running sandbox (make ledger)
sandbox-test:
	go test -count=1 -v -tags=sandbox ./services/ledger/internal/adapter/canton/...

vuln:
	go tool govulncheck ./...

# local Canton sandbox with the test token rail and the demo seed (keep it running)
ledger:
	./scripts/ledger-start.sh

up:
	docker compose up -d --wait postgres redis

up-all:
	$(COMPOSE) up -d --build --wait

down:
	$(COMPOSE) down

# also drops the databases: needed after restarting the in-memory sandbox (old wallets are gone)
clean:
	$(COMPOSE) down -v

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

# service tests against real Postgres and Redis (make up)
integration-test:
	go test -count=1 -tags=integration ./services/...
