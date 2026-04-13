# Hades Makefile
# Hades 高性能代理内核构建脚本

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-s -w -X github.com/hades/hades/internal/version.Version=$(VERSION) -X github.com/hades/hades/internal/version.BuildTime=$(BUILD_TIME)"

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
BINARY_LINUX := $(BIN_DIR)/$(BINARY_NAME)-linux-amd64
BINARY_DARWIN := $(BIN_DIR)/$(BINARY_NAME)-darwin-amd64
BINARY_WINDOWS := $(BIN_DIR)/$(BINARY_NAME)-windows-amd64.exe

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
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_LINUX) $(MAIN_SRC)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DARWIN) $(MAIN_SRC)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_WINDOWS) $(MAIN_SRC)
	@echo "Cross compilation complete!"

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
	@echo "  make build        - 构建当前平台"
	@echo "  make cross-compile - 跨平台编译"
	@echo "  make test         - 运行测试"
	@echo "  make bench        - 性能测试"
	@echo "  make deps         - 安装依赖"
	@echo "  make clean        - 清理构建产物"
	@echo "  make lint         - 代码检查"
	@echo "  make run          - 构建并运行"
