GIT_TAG    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

.PHONY: build test lint generate

build:
	go build -buildvcs=false \
		-ldflags="-X github.com/lstellway/prsm/internal/subcommand.Version=$(GIT_TAG)+$(GIT_COMMIT)" \
		-o prsm ./cmd/prsm

test:
	go test -race ./...

# Mirrors the checks in .github/workflows/ci.yml, so a green `make lint`
# means CI's gofmt/vet steps will be green too.
lint:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "$$unformatted"; \
		exit 1; \
	fi
	go vet ./...
	go vet -tags integration ./...

generate:
	@echo "buf generate (not yet configured)"
