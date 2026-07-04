BINARY     := kriteria
GO_PKG     := ./cmd/kriteria
UI_DIR     := ./ui
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -ldflags "-X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)"

.PHONY: all build ui-build ui-dev run dev clean

## Build everything: frontend then Go binary
all: ui-build build

## Build the Go binary (requires ui-build to have been run)
build:
	go build $(LDFLAGS) -o bin/$(BINARY) $(GO_PKG)

## Build the Vite frontend (outputs to internal/web/dist)
ui-build:
	cd $(UI_DIR) && npm ci && npm run build

## Start Vite dev server only (HMR, proxies API to :8088)
ui-dev:
	cd $(UI_DIR) && npm run dev

## Run the compiled binary
run: all
	./bin/$(BINARY)

## Development mode: start Go backend + Vite dev server concurrently
dev:
	@echo "Starting Go backend on :8088 and Vite dev server on :5173"
	@go run $(GO_PKG) & \
	cd $(UI_DIR) && npm run dev

## Cross-compile for linux/arm64
build-linux-arm64: ui-build
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY)-linux-arm64 $(GO_PKG)

## Remove build artifacts
clean:
	rm -rf bin/ $(UI_DIR)/node_modules $(UI_DIR)/dist internal/web/dist
