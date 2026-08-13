# Native targets (vet/test/build-windows/check) run the toolchain directly.
# They are what executes INSIDE the container, and they work on the host too
# if Go is installed. The docker-* targets are thin wrappers around them.
#
# Never make a native target depend on a docker-* target.

export UID ?= $(shell id -u)
export GID ?= $(shell id -g)

COMPOSE := docker compose
RUN     := $(COMPOSE) run --rm dev

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: check vet test build-windows dist clean \
        docker-check docker-build-windows docker-dist docker-shell \
        docker-image docker-run docker-clean

## --- native targets -------------------------------------------------------

check: vet test build-windows

vet:
	go vet ./...

test:
	go test -race ./...

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
		go build -ldflags "-s -w -H windowsgui" -o dist/mousemover.exe ./cmd/mousemover

# Native release build: full gate, then report what was produced.
dist: check
	@echo "built $(VERSION): dist/mousemover.exe"
	@ls -lh dist/mousemover.exe

clean:
	rm -rf dist

## --- containerised wrappers ----------------------------------------------

docker-image:
	$(COMPOSE) build

docker-check:
	$(RUN) make check

docker-build-windows:
	$(RUN) make build-windows

# Containerised release build — the authoritative one.
docker-dist:
	$(RUN) make dist

docker-shell:
	$(RUN) bash

# Escape hatch for one-off toolchain commands, e.g.
#   make docker-run CMD="go test ./internal/config/ -v"
docker-run:
	$(RUN) $(CMD)

docker-clean:
	$(COMPOSE) down -v --remove-orphans
	rm -rf dist
