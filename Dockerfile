# Build/test environment for mousemover.
#
# This image only ever runs the toolchain — the product is a Windows binary
# cross-compiled out of it, never a container that runs the app. There is
# deliberately no multi-stage "runtime" layer, because there is no Linux
# runtime to produce.
FROM golang:1.26.5-bookworm

# git: `go build` version stamping and `git describe` in the dist target.
# make: the container runs the same native Makefile targets the host does.
# file: used by the build checks to prove the output is a Windows GUI binary.
RUN apt-get update \
 && apt-get install -y --no-install-recommends git make file \
 && rm -rf /var/lib/apt/lists/*

# Caches live on named volumes mounted here (see docker-compose.yml) and must
# be writable by the arbitrary host UID the container runs as.
ENV GOCACHE=/cache/go-build \
    GOMODCACHE=/cache/go-mod \
    GOFLAGS=-buildvcs=false
RUN mkdir -p /cache/go-build /cache/go-mod && chmod -R 777 /cache

WORKDIR /src
