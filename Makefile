APP_NAME := winet2mqtt
BUILD_DIR := ./build
MAIN := ./cmd/winet2mqtt

.PHONY: all build test lint clean docker run

all: lint test build

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME) $(MAIN)

test:
	go test -race -cover ./...

lint:
	golangci-lint run

it-up:
	DOCKER_HOST=unix://$(HOME)/.orbstack/run/docker.sock docker compose -f docker-compose.test.yml up -d --build

it-test: it-up
	@echo "Waiting for services..."
	@sleep 10
	go test -v -tags=integration -count=1 ./test/integration/

it-down:
	DOCKER_HOST=unix://$(HOME)/.orbstack/run/docker.sock docker compose -f docker-compose.test.yml down -v

clean:
	rm -rf $(BUILD_DIR)

docker:
	docker build -t $(APP_NAME):latest .

run: build
	$(BUILD_DIR)/$(APP_NAME) server $(ARGS)

fmt:
	gofmt -w .
	goimports -w .

tidy:
	go mod tidy

.DEFAULT_GOAL := all
