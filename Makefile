.PHONY: build install

BINARY := wacli
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)
CGO_ENABLED ?= 1

build:
	CGO_ENABLED=$(CGO_ENABLED) go build -ldflags "$(LDFLAGS)" -trimpath -o $(BINARY) ./cmd/wacli

install: build
	mkdir -p $(HOME)/.local/bin
	install -Dm755 $(BINARY) $(HOME)/.local/bin/$(BINARY)
