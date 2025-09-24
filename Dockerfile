ARG BUILDER_IMAGE_TAG=ffmpeg-7.1.1-vips-8.17.2-go-1.25.1

# Stage 1: Build application using builder image with go + libvips + FFmpeg
FROM ghcr.io/cshum/imagorvideo-builder:${BUILDER_IMAGE_TAG} AS builder

ENV PKG_CONFIG_PATH=/usr/local/lib/pkgconfig

WORKDIR ${GOPATH}/src/github.com/cshum/imagorvideo

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN go build -o ${GOPATH}/bin/imagorvideo ./cmd/imagorvideo/main.go

# Stage 2: Runtime image
FROM debian:trixie-slim as runtime
LABEL maintainer="adrian@cshum.com"

COPY --from=builder /usr/local/lib /usr/local/lib
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

RUN DEBIAN_FRONTEND=noninteractive \
  apt-get update && \
  apt-get install --no-install-recommends -y \
  procps curl libglib2.0-0 libjpeg62-turbo libpng16-16 libopenexr-3-1-30 \
  libwebp7 libwebpmux3 libwebpdemux2 libtiff6 libexif12 libxml2 libpoppler-glib8t64 \
  libpango-1.0-0 libmatio13 libopenslide0 libopenjp2-7 libjemalloc2 \
  libgsf-1-114 libfftw3-bin liborc-0.4-0 librsvg2-2 libcfitsio10t64 libimagequant0 libaom3 \
  libspng0 libcgif0 libheif1 libheif-plugin-x265 libheif-plugin-aomenc libjxl0.11 libavif-dev \
  libmagickwand-7.q16-10 \
  libdav1d7 libx264-dev libx265-dev libnuma-dev libvpx9 libtheora0 libvorbis-dev && \
  ln -s /usr/lib/$(uname -m)-linux-gnu/libjemalloc.so.2 /usr/local/lib/libjemalloc.so && \
  apt-get autoremove -y && \
  apt-get autoclean && \
  apt-get clean && \
  rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

COPY --from=builder /go/bin/imagorvideo /usr/local/bin/imagorvideo

ENV VIPS_WARNING=0
ENV MALLOC_ARENA_MAX=2
ENV LD_PRELOAD=/usr/local/lib/libjemalloc.so

ENV PORT 8000

# use unprivileged user
USER nobody

ENTRYPOINT ["/usr/local/bin/imagorvideo"]

EXPOSE ${PORT}
