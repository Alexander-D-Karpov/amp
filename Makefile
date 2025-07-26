.PHONY: build run test lint clean install-deps cross-platform setup-dev help

APP_NAME = amp
DESKTOP_CMD = ./cmd/desktop
MOBILE_CMD = ./cmd/mobile
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -ldflags "-X main.Version=$(VERSION)"

help:
	@echo "Available targets:"
	@echo "  build-desktop    Build desktop application"
	@echo "  build-mobile     Build mobile application"
	@echo "  run-desktop      Run desktop application"
	@echo "  run-mobile       Run mobile application"
	@echo "  test            Run all tests"
	@echo "  lint            Run linter"
	@echo "  lint-fix        Run linter with auto-fix"
	@echo "  clean           Clean build artifacts"
	@echo "  setup-dev       Setup development environment"
	@echo "  cross-platform  Build for all platforms"

build-desktop:
	@echo "Building desktop application..."
	@mkdir -p bin
	cd $(DESKTOP_CMD) && fyne build -o ../../bin/$(APP_NAME)

build-mobile:
	@echo "Building mobile application..."
	cd $(MOBILE_CMD) && fyne package -os android

run-desktop:
	@echo "Running desktop application..."
	cd $(DESKTOP_CMD) && go run $(LDFLAGS) main.go

run-mobile:
	@echo "Running mobile application..."
	cd $(MOBILE_CMD) && fyne run -os android

test:
	@echo "Running tests..."
	go test -v -race -cover ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	@echo "Running linter..."
	golangci-lint run

lint-fix:
	@echo "Running linter with auto-fix..."
	golangci-lint run --fix

clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -rf fyne-cross/
	rm -rf dist/
	rm -f *.apk *.exe *.app *.tar.xz
	rm -f coverage.out coverage.html

install-deps:
	@echo "Installing dependencies..."
	go mod download
	go mod verify
	@echo "Installing development tools..."
	go install fyne.io/fyne/v2/cmd/fyne@latest
	go install github.com/fyne-io/fyne-cross@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

setup-dev: install-deps
	@echo "Setting up development environment..."
	mkdir -p bin data cache configs logs assets
	@if [ ! -f configs/config.yaml ]; then \
		echo "Creating default config file..."; \
		cp configs/config.example.yaml configs/config.yaml; \
	fi
	@if [ ! -f assets/icon.png ]; then \
		echo "Please add an icon file at assets/icon.png"; \
	fi

cross-platform:
	@echo "Building for all platforms..."
	mkdir -p dist
	cd $(DESKTOP_CMD) && fyne-cross windows -arch amd64 -output ../../dist/
	cd $(DESKTOP_CMD) && fyne-cross darwin -arch amd64 -output ../../dist/
	cd $(DESKTOP_CMD) && fyne-cross linux -arch amd64 -output ../../dist/
	cd $(MOBILE_CMD) && fyne-cross android -arch arm64 -output ../../dist/

package-windows:
	@echo "Packaging for Windows..."
	cd $(DESKTOP_CMD) && fyne package -os windows

package-linux:
	@echo "Packaging for Linux..."
	cd $(DESKTOP_CMD) && fyne package -os linux

package-darwin:
	@echo "Packaging for macOS..."
	cd $(DESKTOP_CMD) && fyne package -os darwin

package-android:
	@echo "Packaging for Android..."
	cd $(MOBILE_CMD) && fyne package -os android

dev-run: setup-dev run-desktop

dev-test: lint test

dev-build: clean build-desktop

release-build: clean setup-dev cross-platform
	@echo "Release build completed in dist/"

docker-build:
	docker build -t $(APP_NAME):$(VERSION) .

docs:
	@echo "Generating documentation..."
	go doc -all ./... > docs/API.md

all: clean setup-dev release-build