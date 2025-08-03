.PHONY: build run test lint clean install-deps cross-platform setup-dev setup-check help bundle

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
	@echo "  bundle           Bundle resources"
	@echo "  test            Run all tests"
	@echo "  lint            Run linter"
	@echo "  lint-fix        Run linter with auto-fix"
	@echo "  clean           Clean build artifacts"
	@echo "  setup-dev       Setup development environment"
	@echo "  cross-platform  Build for all platforms"

bundle:
	@echo "Bundling resources..."
	@rm -f internal/ui/bundle.go
	@if [ -f "assets/icon.png" ]; then \
		fyne bundle -package ui -o internal/ui/bundle.go assets/icon.png; \
		echo "Bundled assets/icon.png"; \
	elif [ -f "Icon.png" ]; then \
		fyne bundle -package ui -o internal/ui/bundle.go Icon.png; \
		echo "Bundled Icon.png"; \
	else \
		echo "package ui" > internal/ui/bundle.go; \
		echo "" >> internal/ui/bundle.go; \
		echo "// No icon files found" >> internal/ui/bundle.go; \
	fi

build-desktop: bundle
	@echo "Building desktop application..."
	@mkdir -p bin
	cd $(DESKTOP_CMD) && fyne build -o ../../bin/$(APP_NAME)

build-mobile:
	@echo "Building mobile application..."
	cd $(MOBILE_CMD) && fyne package -os android

run-desktop: bundle
	@echo "Running desktop application..."
	cd $(DESKTOP_CMD) && go run $(LDFLAGS) main.go

run-mobile:
	@echo "Running mobile application..."
	cd $(MOBILE_CMD) && go run $(LDFLAGS) main.go

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
	rm -f internal/ui/bundle.go

install-deps:
	@echo "Installing dependencies..."
	go mod download
	go mod verify
	@echo "Installing development tools..."
	go install fyne.io/fyne/v2/cmd/fyne@latest
	go install github.com/fyne-io/fyne-cross@latest
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint already installed"; \
	else \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi

setup-dev: install-deps
	@echo "Setting up development environment..."
	mkdir -p bin data cache configs logs assets internal/ui
	@if [ ! -f configs/config.yaml ]; then \
		echo "Creating default config file..."; \
		cp configs/config.example.yaml configs/config.yaml; \
	fi
	@if [ ! -f assets/icon.png ]; then \
		echo "Please add an icon file at assets/icon.png"; \
	fi

cross-platform: bundle
	@echo "Building for all platforms..."
	mkdir -p dist
	cd $(DESKTOP_CMD) && fyne-cross windows -arch amd64 -output ../../dist/
	cd $(DESKTOP_CMD) && fyne-cross darwin -arch amd64 -output ../../dist/
	cd $(DESKTOP_CMD) && fyne-cross linux -arch amd64 -output ../../dist/
	cd $(MOBILE_CMD) && fyne-cross android -arch arm64 -output ../../dist/

package-windows: bundle
	@echo "Packaging for Windows..."
	cd $(DESKTOP_CMD) && fyne package -os windows

package-linux: bundle
	@echo "Packaging for Linux..."
	cd $(DESKTOP_CMD) && fyne package -os linux

package-darwin: bundle
	@echo "Packaging for macOS..."
	cd $(DESKTOP_CMD) && fyne package -os darwin

package-android:
	@echo "Packaging for Android..."
	cd $(MOBILE_CMD) && fyne package -os android

setup-check:
	@if [ ! -d "data" ] || [ ! -d "cache" ] || [ ! -f "configs/config.yaml" ]; then \
		echo "Running initial setup..."; \
		$(MAKE) setup-dev; \
	else \
		echo "Development environment already set up"; \
	fi

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