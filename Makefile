# Hades Makefile
# Hades 高性能代理内核构建脚本

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GOVERSION := $(shell $(GOCMD) version 2>/dev/null | awk '{print $$3}' || echo "go1.25.0")
LDFLAGS := -ldflags "-s -w -X github.com/Qing060325/Hades/internal/version.Version=$(VERSION) -X github.com/Qing060325/Hades/internal/version.BuildTime=$(BUILD_TIME) -X github.com/Qing060325/Hades/internal/version.GoVersion=$(GOVERSION)"

# Go 参数
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod

# 输出目录
BIN_DIR := bin
MAIN_SRC := ./cmd/hades

# 目标文件
BINARY_NAME := hades
BINARY_LINUX_AMD64 := $(BIN_DIR)/$(BINARY_NAME)-linux-amd64
BINARY_LINUX_ARM64 := $(BIN_DIR)/$(BINARY_NAME)-linux-arm64
BINARY_DARWIN_AMD64 := $(BIN_DIR)/$(BINARY_NAME)-darwin-amd64
BINARY_DARWIN_ARM64 := $(BIN_DIR)/$(BINARY_NAME)-darwin-arm64
BINARY_WINDOWS_AMD64 := $(BIN_DIR)/$(BINARY_NAME)-windows-amd64.exe

# OpenWrt 目标
BINARY_MIPS := $(BIN_DIR)/$(BINARY_NAME)-linux-mips
BINARY_MIPSLE := $(BIN_DIR)/$(BINARY_NAME)-linux-mipsle
BINARY_MIPS64LE := $(BIN_DIR)/$(BINARY_NAME)-linux-mips64le
BINARY_ARMV7 := $(BIN_DIR)/$(BINARY_NAME)-linux-armv7
BINARY_ARM64 := $(BIN_DIR)/$(BINARY_NAME)-linux-arm64

.PHONY: all build clean test deps cross-compile

all: deps build

## 构建当前平台
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME) $(MAIN_SRC)

## 跨平台编译
cross-compile:
	@echo "Cross compiling..."
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_LINUX_AMD64) $(MAIN_SRC)
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_LINUX_ARM64) $(MAIN_SRC)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DARWIN_AMD64) $(MAIN_SRC)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DARWIN_ARM64) $(MAIN_SRC)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_WINDOWS_AMD64) $(MAIN_SRC)
	@echo "Cross compilation complete!"

## OpenWrt 交叉编译（所有常见架构）
openwrt:
	@echo "Cross compiling for OpenWrt..."
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=mips GOMIPS=softfloat $(GOBUILD) $(LDFLAGS) -o $(BINARY_MIPS) $(MAIN_SRC)
	GOOS=linux GOARCH=mipsle GOMIPS=softfloat $(GOBUILD) $(LDFLAGS) -o $(BINARY_MIPSLE) $(MAIN_SRC)
	GOOS=linux GOARCH=mips64le $(GOBUILD) $(LDFLAGS) -o $(BINARY_MIPS64LE) $(MAIN_SRC)
	GOOS=linux GOARCH=arm GOARM=7 $(GOBUILD) $(LDFLAGS) -o $(BINARY_ARMV7) $(MAIN_SRC)
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_ARM64) $(MAIN_SRC)
	@echo "OpenWrt compilation complete!"
	@ls -lh $(BIN_DIR)/$(BINARY_NAME)-linux-mips*

## OpenWrt 单架构编译
openwrt-mips:
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=mips GOMIPS=softfloat $(GOBUILD) $(LDFLAGS) -o $(BINARY_MIPS) $(MAIN_SRC)
	@echo "Built: $(BINARY_MIPS)"

openwrt-mipsle:
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=mipsle GOMIPS=softfloat $(GOBUILD) $(LDFLAGS) -o $(BINARY_MIPSLE) $(MAIN_SRC)
	@echo "Built: $(BINARY_MIPSLE)"

openwrt-arm64:
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_ARM64) $(MAIN_SRC)
	@echo "Built: $(BINARY_ARM64)"

## 运行测试
test:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

## 查看测试覆盖率
coverage: test
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## 安装依赖
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

## 清理构建产物
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BIN_DIR)
	rm -f coverage.out coverage.html

## 代码检查
lint:
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run ./...

## 性能测试
bench:
	$(GOTEST) -bench=. -benchmem ./...

## 运行
run: build
	./$(BIN_DIR)/$(BINARY_NAME) -c configs/config.yaml

## 帮助
help:
	@echo "可用目标:"
	@echo "  make build          - 构建当前平台"
	@echo "  make cross-compile  - 跨平台编译 (linux/mac/windows amd64+arm64)"
	@echo "  make openwrt        - OpenWrt 全架构编译 (mips/mipsle/mips64le/armv7/arm64)"
	@echo "  make openwrt-mips   - OpenWrt MIPS 编译"
	@echo "  make openwrt-mipsle - OpenWrt MIPSLE 编译"
	@echo "  make openwrt-arm64  - OpenWrt ARM64 编译"
	@echo "  make test           - 运行测试"
	@echo "  make bench          - 性能测试"
	@echo "  make deps           - 安装依赖"
	@echo "  make clean          - 清理构建产物"
	@echo "  make lint           - 代码检查"
	@echo "  make run            - 构建并运行"
