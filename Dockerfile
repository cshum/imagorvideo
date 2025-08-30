ARG GOLANG_VERSION=1.25.0
FROM golang:${GOLANG_VERSION}-trixie as builder

ARG FFMPEG_VERSION=7.1.1
ARG VIPS_VERSION=8.17.1
ARG TARGETARCH

ENV PKG_CONFIG_PATH=/usr/local/lib/pkgconfig
ENV MAKEFLAGS="-j8"

# Installs libvips + FFmpeg + required libraries including modern image formats + ImageMagick
RUN DEBIAN_FRONTEND=noninteractive \
  apt-get update && \
  apt-get install --no-install-recommends -y \
  ca-certificates \
  automake build-essential curl \
  meson ninja-build pkg-config \
  gobject-introspection gtk-doc-tools libglib2.0-dev libjpeg62-turbo-dev libpng-dev \
  libwebp-dev libtiff-dev libexif-dev libxml2-dev libpoppler-glib-dev \
  swig libpango1.0-dev libmatio-dev libopenslide-dev libcfitsio-dev libopenjp2-7-dev liblcms2-dev \
  libgsf-1-dev libfftw3-dev liborc-0.4-dev librsvg2-dev libimagequant-dev libaom-dev \
  libspng-dev libcgif-dev libheif-dev libheif-plugin-x265 libheif-plugin-aomenc libjxl-dev libavif-dev \
  libmagickwand-dev \
  yasm libx264-dev libx265-dev libnuma-dev libvpx-dev libtheora-dev  \
  librtmp-dev libvorbis-dev libdav1d-dev && \
  cd /tmp && \
    curl -fsSLO https://github.com/libvips/libvips/releases/download/v${VIPS_VERSION}/vips-${VIPS_VERSION}.tar.xz && \
    tar xf vips-${VIPS_VERSION}.tar.xz && \
    cd vips-${VIPS_VERSION} && \
    meson setup _build \
    --buildtype=release \
    --strip \
    --prefix=/usr/local \
    --libdir=lib \
    -Dmagick=enabled \
    -Djpeg-xl=enabled \
    -Dintrospection=disabled && \
    ninja -C _build && \
    ninja -C _build install && \
  cd /tmp && \
    curl -fsSLO https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz && \
    tar xf ffmpeg-${FFMPEG_VERSION}.tar.xz && \
    cd /tmp/ffmpeg-${FFMPEG_VERSION} && \
    ./configure --prefix=/usr/local  \
    --disable-debug  \
    --disable-doc  \
    --disable-ffplay \
    --disable-static  \
    --enable-shared  \
    --enable-version3  \
    --enable-gpl  \
    --enable-libtheora \
    --enable-libvorbis \
    --enable-librtmp \
    --enable-libwebp \
    --enable-libvpx  \
    --enable-libx265  \
    --enable-libx264 \
    --enable-libdav1d \
    --enable-libaom && \
    make && make install && \
  ldconfig && \
  rm -rf /usr/local/lib/python* && \
  rm -rf /usr/local/lib/libvips-cpp.* && \
  rm -rf /usr/local/lib/*.a && \
  rm -rf /usr/local/lib/*.la

WORKDIR ${GOPATH}/src/github.com/cshum/imagorvideo

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN if [ "$TARGETARCH" = "amd64" ]; then go test ./...; fi
RUN go build -o ${GOPATH}/bin/imagorvideo ./cmd/imagorvideo/main.go

FROM debian:trixie-slim as runtime
LABEL maintainer="adrian@cshum.com"

COPY --from=builder /usr/local/lib /usr/local/lib
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

# Install runtime dependencies including modern image formats and ImageMagick
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
