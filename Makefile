VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build run test lint check

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/airpeek ./cmd/airpeek

# make run ARGS="--env local"
run:
	go run ./cmd/airpeek $(ARGS)

test:
	go test -race ./...

lint:
	go vet ./...
	golangci-lint run

check: lint test