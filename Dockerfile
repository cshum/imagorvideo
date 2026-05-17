ARG GOLANG_VERSION=1.26.1
ARG BASE_IMAGE=ghcr.io/cshum/imagor-base:vips8.18.2-r5-magick-ffmpeg
ARG DEV_BASE_IMAGE=${BASE_IMAGE}-dev

FROM golang:${GOLANG_VERSION}-bookworm AS golang-base

FROM ${BASE_IMAGE} AS native-base

# Stage 1: Build application using imagor-base ffmpeg+magick dev image
FROM ${DEV_BASE_IMAGE} AS builder

COPY --from=golang-base /usr/local/go /usr/local/go

ENV GOPATH=/go
ENV PATH=/usr/local/go/bin:/go/bin:$PATH
ENV CGO_ENABLED=1
ENV PKG_CONFIG_PATH=/opt/imagor/lib/pkgconfig
ENV CGO_CFLAGS=-I/opt/imagor/include
ENV CGO_LDFLAGS="-L/opt/imagor/lib -Wl,-rpath,/opt/imagor/lib"

WORKDIR ${GOPATH}/src/github.com/cshum/imagorvideo

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN go build -o /opt/imagor/bin/imagorvideo ./cmd/imagorvideo/main.go

# Stage 2: Runtime image
FROM native-base AS runtime
LABEL maintainer="adrian@cshum.com"

RUN apt-get update \
  && apt-get upgrade -y \
  && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    curl \
    media-types \
    procps \
  && ln -s /usr/lib/$(uname -m)-linux-gnu/libjemalloc.so.2 /usr/local/lib/libjemalloc.so \
  && mkdir -p /var/cache/fontconfig \
  && chmod 777 /var/cache/fontconfig \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /opt/imagor/bin/imagorvideo /usr/local/bin/imagorvideo

ENV VIPS_WARNING=0
ENV MALLOC_ARENA_MAX=2
ENV LD_PRELOAD=/usr/local/lib/libjemalloc.so
ENV XDG_CACHE_HOME=/tmp

ENV PORT 8000

# use unprivileged user
USER nobody

ENTRYPOINT ["/usr/local/bin/imagorvideo"]

EXPOSE ${PORT}
