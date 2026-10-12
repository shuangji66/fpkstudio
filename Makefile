# Makefile: builds the Vue frontend, embeds the static assets into the Go
# backend binary, then compiles the single self-contained fpkstudio backend.
#
#   make                   # dev (default, non-stripped)
#   make release V=1.0.1   # release build (stripped), version override
#   make test              # backend unit tests
#   make clean             # remove build artifacts
#
# The backend serves the embedded assets over its unix socket under the
# configured baseurl (e.g. /app/fpkstudio) fronted by nginx.
#
# Go cache/module variables default to project-local dirs; pre-set environment
# values (GOCACHE/GOPATH) still take precedence.

ROOT      := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
BACKEND   := $(ROOT)/backend
FRONTEND  := $(ROOT)/frontend
EMBED     := $(BACKEND)/embed
OUT       := $(ROOT)/fpkstudio

# 版本号：默认 1.0.0，可经 V 覆盖（如 make release V=1.0.1）。
version   ?= 1.0.0
V         ?= $(version)

# 默认把 Go 缓存放到项目本地目录；若环境中已设置 GOCACHE/GOPATH 则沿用环境值
# （?= 只在未定义时赋值）。
GOCACHE   ?= $(ROOT)/.gocache
GOPATH    ?= $(ROOT)/.gopath
# go build 的工作目录走 os.TempDir()，也就是 $TMPDIR 或 /tmp。有些环境（含飞牛的
# 存储卷）下 /tmp 权限受限，构建会报 "open /tmp/go-build.../exe/a.out: permission
# denied"，指到项目本地即可。
TMPDIR    ?= $(ROOT)/.tmp
export GOCACHE GOPATH TMPDIR

# 本程序是纯 Go，不需要 cgo。关掉它有两个好处：产物静态链接（真正的单文件交付），
# 而且冷编译不再走 runtime/cgo —— 那一步会写 .o 中间文件，在本项目的构建卷上会被
# 损坏，报 "cannot parse $WORK/.../_cgo_.o as ELF"。环境里已设 CGO_ENABLED 则沿用。
CGO_ENABLED ?= 0
export CGO_ENABLED

# -buildvcs=false：仓库可能没有 .git，或者构建环境不允许读它；没必要因此失败。
# 保留调用者已经设置的其它 GOFLAGS。
export GOFLAGS := -buildvcs=false $(GOFLAGS)
# fnOS 上 Node 由应用提供，不一定在 PATH 里。取版本号最高的那一个，可以用 NODE_BIN 覆盖；
# 一个都没有（别的构建机）就什么都不加——不能退化成 `PATH=:$(PATH)`，那等于把当前目录
# 塞进 PATH 里。
NODE_BIN ?= $(shell ls -d /var/apps/nodejs_v*/target/bin 2>/dev/null | sort -V | tail -1)
ifneq ($(strip $(NODE_BIN)),)
export PATH := $(NODE_BIN):$(PATH)
endif

.PHONY: all dev release test clean

all: dev

# Frontend build artifacts / node_modules flagged in ROOT/.gitignore.
.NOTPARALLEL:

dev: ## Dev build (default, non-stripped)
	$(MAKE) build LDFLAGS="-X main.version=$(V)"

release: ## Release build (strip symbols)
	$(MAKE) build LDFLAGS="-s -w -X main.version=$(V)"

# 后端单元测试。main.go 用 //go:embed all:embed 打包前端产物，而该目录在构建后
# 被清空，因此测试前先补一个占位文件（否则包无法编译）。占位内容不影响测试。
test: ## Run backend unit tests
	mkdir -p "$(EMBED)"
	@test -e "$(EMBED)/index.html" || echo "placeholder for //go:embed (see Makefile test target)" > "$(EMBED)/index.html"
	cd "$(BACKEND)" && go test ./...

# Common build steps. LDFLAGS is passed in from dev/release.
build:
	@echo "==> Building frontend..."
	cd "$(FRONTEND)" && npm install --no-audit --no-fund && npm run build
	@echo "==> Copying frontend dist into embed dir..."
	rm -rf "$(EMBED)"
	mkdir -p "$(EMBED)"
	tar -C "$(FRONTEND)/dist" -cf - . | tar -C "$(EMBED)" -xf -
	@echo "==> Building Go binary..."
	# Ensure local Go cache dirs exist。
	mkdir -p "$(GOCACHE)" "$(GOPATH)" "$(TMPDIR)"
	# Clean the Go build cache so code and embedded-asset changes always compile in.
	go clean -cache
	cd "$(BACKEND)" && go build -trimpath -ldflags "$(LDFLAGS)" -o "$(OUT)" .
	# 显式补一次执行位：飞牛的存储卷上新文件的模式位不可靠，不补的话拿到的可能是一个
	# 跑不起来的 0000 文件。
	chmod 755 "$(OUT)"
	@echo "==> Cleaning embed dir..."
	rm -rf "$(EMBED)"
	@echo "==> Done: $(OUT)"
	ls -lh "$(OUT)"

clean:
	rm -rf "$(EMBED)"
	rm -f "$(OUT)"
