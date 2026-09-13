VERSION ?= dev
PREFIX ?= $(HOME)/.local

.PHONY: build install test vet fmt

build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/plane ./cmd/plane

install: build
	install -Dm755 bin/plane $(PREFIX)/bin/plane

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal
