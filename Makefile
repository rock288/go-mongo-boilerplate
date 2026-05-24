.PHONY: run worker build mocks wire test test-cov test-race lint vuln secrets ci init \
        migrate-up migrate-down migrate-version hooks scaffold \
        docker-build docker-scan signoz-up signoz-down \
        localstack-up localstack-down sqs-create-queues tidy clean

MODULE  := $(shell go list -m)
BIN_DIR := bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
IMAGE   ?= go-mongo-boilerplate
GOLANGCI_LINT_VERSION ?= v2.1.0
GOLANGCI_LINT := $(shell go env GOPATH)/bin/golangci-lint

LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

run:
	go run ./cmd/server

# feature:worker:start
worker:
	go run ./cmd/worker
# feature:worker:end

# Interactive bootstrap CLI. Self-destructs after a successful run.
init:
	cd cmd/init && go run . --root=../..

build:
	mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/server ./cmd/server
# feature:worker:start
	go build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/worker ./cmd/worker
# feature:worker:end
	go build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/migrate ./cmd/migrate

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-version:
	go run ./cmd/migrate version

wire:
	cd cmd/server && wire
# feature:worker:start
	cd cmd/worker && wire
# feature:worker:end

mocks:
	mockery

test:
	go test ./... -coverprofile=coverage.out -covermode=atomic

test-cov: test
	go tool cover -html=coverage.out -o coverage.html

test-race:
	go test -race -timeout=5m ./...

$(GOLANGCI_LINT):
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run

vuln:
	go install golang.org/x/vuln/cmd/govulncheck@latest
	$(shell go env GOPATH)/bin/govulncheck ./...

secrets:
	@command -v gitleaks >/dev/null || (echo "install gitleaks: brew install gitleaks"; exit 1)
	gitleaks detect --no-banner --redact

docker-build:
	docker build --build-arg TARGET=server  --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t $(IMAGE)-server:$(VERSION) .
# feature:worker:start
	docker build --build-arg TARGET=worker  --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t $(IMAGE)-worker:$(VERSION) .
# feature:worker:end
	docker build --build-arg TARGET=migrate --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t $(IMAGE)-migrate:$(VERSION) .

docker-scan:
	@command -v trivy >/dev/null || (echo "install trivy: brew install aquasecurity/trivy/trivy"; exit 1)
	trivy image --severity CRITICAL,HIGH --exit-code 1 $(IMAGE)-server:$(VERSION)

# feature:observability:start
signoz-up:
	docker-compose -f docker-compose.signoz.yml up -d

signoz-down:
	docker-compose -f docker-compose.signoz.yml down
# feature:observability:end

# feature:sqs:start
localstack-up:
	docker compose -f docker-compose.localstack.yml up -d

localstack-down:
	docker compose -f docker-compose.localstack.yml down

# Bootstrap dev SQS queues on LocalStack. Production: manage via Terraform/CDK.
sqs-create-queues:
	scripts/sqs-create-dev-queues.sh
# feature:sqs:end

ci: lint test test-race vuln secrets

hooks:
	@command -v lefthook >/dev/null || (echo "install lefthook: brew install lefthook"; exit 1)
	lefthook install

# scaffold name=Order  →  internal/order/ cloned from internal/user/ with rename
scaffold:
	@test -n "$(name)" || (echo "usage: make scaffold name=<PascalCase>"; exit 2)
	scripts/scaffold.sh $(name)

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR) coverage.out coverage.html
