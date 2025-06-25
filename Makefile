# Root-level Makefile

APP_NAME = kernelkoala
BIN_DIR = bin
BPF_ROOT = bpf

GOARCHS = amd64 arm64 riscv64
GOOS = linux
CURRENT_ARCH = $(shell go env GOARCH)
CURRENT_OS = $(shell go env GOOS)
CURRENT_BIN = $(BIN_DIR)/$(APP_NAME)-$(CURRENT_ARCH)

# 🛠️ Interface name passed via CLI (default lo for dev)
IFACE ?= lo

.PHONY: all dev test build-bpf build-dev run prod clean install

## === Default Target ===
all: dev

## === Dev Build (local arch, tests, bpf, and binary) ===
dev: test build-bpf build-dev

## ✅ Install dependencies (Go + BPF tools)
install:
	@echo "📥 Running install script..."
	@bash ./scripts/install.sh

## ✅ Run Go tests
test:
	@echo "🧪 Running Go tests..."
	go test ./... -v

## 🔨 Build all BPF modules in bpf/*/
build-bpf:
	@echo "🔧 Building all eBPF modules in $(BPF_ROOT)/"
	@for dir in $(BPF_ROOT)/*; do \
		if [ -f $$dir/Makefile ]; then \
			echo "📦 Building BPF module: $$dir"; \
			$(MAKE) -C $$dir; \
		fi \
	done

## 🧪 Local Go binary build for current arch
build-dev:
	@echo "🛠️  Building $(APP_NAME) for $(CURRENT_OS)/$(CURRENT_ARCH)..."
	@mkdir -p $(BIN_DIR)
	GOOS=$(CURRENT_OS) GOARCH=$(CURRENT_ARCH) go build -o $(CURRENT_BIN) ./cmd
	@echo "✅ Built: $(CURRENT_BIN)"

## 🚀 Run the app with interface (default: lo, override with `IFACE=eth0`)
run: dev
	@echo "🚀 Running $(CURRENT_BIN) on interface: $(IFACE)"
	@sudo ENV=dev $(CURRENT_BIN) $(IFACE)

## 🚀 Production multi-arch Go build
prod: clean build-bpf
	@echo "🏗️  Building production Go binaries..."
	@mkdir -p $(BIN_DIR)
	@for arch in $(GOARCHS); do \
		echo "🔧 GOARCH=$$arch"; \
		GOOS=$(GOOS) GOARCH=$$arch go build -o $(BIN_DIR)/$(APP_NAME)-$$arch ./cmd; \
	done
	@echo "✅ All binaries available in $(BIN_DIR)/"

## 🧹 Clean binaries and BPF builds
clean:
	@echo "🧹 Cleaning binaries and BPF builds..."
	rm -rf $(BIN_DIR)
	@for dir in $(BPF_ROOT)/*; do \
		if [ -f $$dir/Makefile ]; then \
			echo "🧼 Cleaning $$dir"; \
			$(MAKE) -C $$dir clean; \
		fi \
	done
