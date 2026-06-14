GIT_TAG    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

.PHONY: build test lint generate

build:
	go build -buildvcs=false \
		-ldflags="-X github.com/lstellway/prsm/internal/subcommand.Version=$(GIT_TAG)+$(GIT_COMMIT)" \
		-o prsm ./cmd/prsm

test:
	go test ./...

lint:
	go vet ./...

generate:
	@echo "buf generate (not yet configured)"
