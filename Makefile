SHELL := /bin/sh

GO ?= go
GOTOOLCHAIN ?= auto
PKG := ./...
BIN_DIR := bin
BIN := $(BIN_DIR)/foco
BIN_PURE := $(BIN_DIR)/foco-pure

PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
DESTDIR ?=
INSTALL ?= install
GOENV := GOTOOLCHAIN=$(GOTOOLCHAIN)

CGO_ENABLED ?= 1
GO_TAGS ?= vectorscan

ifeq ($(CGO_ENABLED),0)
GO_TAGS :=
endif

ifneq ($(GO_TAGS),)
TAGS_FLAG := -tags "$(GO_TAGS)"
else
TAGS_FLAG :=
endif

VECTORSCAN_PREFIX := $(shell command -v brew >/dev/null 2>&1 && brew --prefix vectorscan 2>/dev/null)
ifneq ($(VECTORSCAN_PREFIX),)
export PKG_CONFIG_PATH := $(VECTORSCAN_PREFIX)/lib/pkgconfig:$(PKG_CONFIG_PATH)
endif

.PHONY: all build build-pure install install-pure uninstall deps check-vectorscan verify-vectorscan check ci fmt fmt-check vet test test-race test-vectorscan test-race-vectorscan tidy clean help

all: check build

build: | $(BIN_DIR)
	FOCO_CGO="$(CGO_ENABLED)" FOCO_TAGS="$(GO_TAGS)" BIN="$(BIN)" GO="$(GO)" GOTOOLCHAIN="$(GOTOOLCHAIN)" sh scripts/build.sh build

build-pure: | $(BIN_DIR)
	CGO_ENABLED=0 $(GOENV) $(GO) build -trimpath -o $(BIN_PURE) ./cmd/cli

install:
	@PROJECT="$(CURDIR)" MODE=accelerated BIN="$(BIN)" BIN_PURE="$(BIN_PURE)" PREFIX="$(PREFIX)" BINDIR="$(BINDIR)" DESTDIR="$(DESTDIR)" INSTALL="$(INSTALL)" CGO_ENABLED="$(CGO_ENABLED)" GO_TAGS="$(GO_TAGS)" GO="$(GO)" GOTOOLCHAIN="$(GOTOOLCHAIN)" sh scripts/install.sh

install-pure:
	@PROJECT="$(CURDIR)" MODE=pure BIN="$(BIN)" BIN_PURE="$(BIN_PURE)" PREFIX="$(PREFIX)" BINDIR="$(BINDIR)" DESTDIR="$(DESTDIR)" INSTALL="$(INSTALL)" CGO_ENABLED="$(CGO_ENABLED)" GO_TAGS="$(GO_TAGS)" GO="$(GO)" GOTOOLCHAIN="$(GOTOOLCHAIN)" sh scripts/install.sh

uninstall:
	rm -f "$(DESTDIR)$(BINDIR)/foco"

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

check: fmt-check vet test

ci: check test-race verify-vectorscan build

fmt:
	gofmt -w .

fmt-check:
	@files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then \
		printf '%s\n' "$$files"; \
		printf '%s\n' "gofmt required; run make fmt"; \
		exit 1; \
	fi

vet:
	$(GOENV) $(GO) vet $(PKG)

test:
	CGO_ENABLED=0 $(GOENV) $(GO) test -count=1 $(PKG)

test-race:
	CGO_ENABLED=1 $(GOENV) $(GO) test -race -count=1 $(PKG)

test-vectorscan: check-vectorscan
	CGO_ENABLED=1 $(GOENV) $(GO) test $(TAGS_FLAG) $(PKG)

test-race-vectorscan: check-vectorscan
	CGO_ENABLED=1 $(GOENV) $(GO) test -race $(TAGS_FLAG) $(PKG)

verify-vectorscan: check-vectorscan
	CGO_ENABLED=1 $(GOENV) $(GO) test -tags "vectorscan" -count=1 -run TestTitusDetectsKnownToken -v ./internal/detect

tidy:
	$(GOENV) $(GO) mod tidy

check-vectorscan:
	@sh scripts/build.sh check

deps:
	@sh scripts/build.sh deps

clean:
	rm -rf $(BIN_DIR)

help:
	@printf '%s\n' \
		'make                        check + build (auto-installs Vectorscan or falls back to pure Go)' \
		'make build                  Build bin/foco (auto-installs Vectorscan or falls back)' \
		'make build-pure             Build bin/foco-pure with the pure-Go fallback' \
		'make install                Install foco to $(BINDIR)' \
		'make install-pure           Install the pure-Go binary to $(BINDIR)' \
		'make uninstall              Remove foco from $(BINDIR)' \
		'make deps                   Install pkg-config + Hyperscan/Vectorscan' \
		'make check-vectorscan       Verify pkg-config can find libhs' \
		'make verify-vectorscan      Prove Titus compiles rules with Hyperscan/Vectorscan' \
		'make check                  gofmt check + go vet + pure-Go tests' \
		'make ci                     check + race + vectorscan verification + build' \
		'make fmt                    Format all Go files' \
		'make test                   Pure-Go test suite' \
		'make test-race              Pure-Go tests with the race detector' \
		'make test-vectorscan        Test suite with the vectorscan build tag' \
		'make test-race-vectorscan   Race tests with the vectorscan build tag' \
		'make tidy                   go mod tidy' \
		'make clean                  Remove bin/'
