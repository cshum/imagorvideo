ARG GOLANG_VERSION=1.21.4
FROM golang:${GOLANG_VERSION}-bookworm as builder

ARG FFMPEG_VERSION=5.1.2
ARG VIPS_VERSION=8.14.3
ARG TARGETARCH

ENV PKG_CONFIG_PATH=/usr/local/lib/pkgconfig
ENV MAKEFLAGS="-j8"


# Installs libvips + required libraries
RUN DEBIAN_FRONTEND=noninteractive \
  apt-get update && \
  apt-get install --no-install-recommends -y \
  ca-certificates \
  automake build-essential curl \
  meson ninja-build pkg-config \
  gobject-introspection gtk-doc-tools libglib2.0-dev libjpeg62-turbo-dev libpng-dev \
  libwebp-dev libtiff-dev libexif-dev libxml2-dev libpoppler-glib-dev \
  swig libpango1.0-dev libmatio-dev libopenslide-dev libcfitsio-dev libopenjp2-7-dev \
  libgsf-1-dev libfftw3-dev liborc-0.4-dev librsvg2-dev libimagequant-dev libaom-dev libheif-dev \
  yasm libx264-dev libx265-dev libnuma-dev libvpx-dev libtheora-dev  \
  libspng-dev libcgif-dev librtmp-dev libvorbis-dev && \
  cd /tmp && \
    curl -fsSLO https://github.com/libvips/libvips/releases/download/v${VIPS_VERSION}/vips-${VIPS_VERSION}.tar.xz && \
    tar xf vips-${VIPS_VERSION}.tar.xz && \
    cd vips-${VIPS_VERSION} && \
    meson setup _build \
    --buildtype=release \
    --strip \
    --prefix=/usr/local \
    --libdir=lib \
    -Dgtk_doc=false \
    -Dmagick=disabled \
    -Dintrospection=false && \
    ninja -C _build && \
    ninja -C _build install && \
  cd /tmp && \
    curl -fsSLO https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.bz2 && \
    tar jvxf ffmpeg-${FFMPEG_VERSION}.tar.bz2 && \
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
    --enable-libx264 && \
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

FROM debian:bookworm-slim
LABEL maintainer="adrian@cshum.com"

COPY --from=builder /usr/local/lib /usr/local/lib
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

# Install runtime dependencies
RUN DEBIAN_FRONTEND=noninteractive \
  apt-get update && \
  apt-get install --no-install-recommends -y \
  procps libglib2.0-0 libjpeg62-turbo libpng16-16 libopenexr-3-1-30 \
  libwebp7 libwebpmux3 libwebpdemux2 libtiff6 libexif12 libxml2 libpoppler-glib8 \
  libpango1.0-0 libmatio11 libopenslide0 libopenjp2-7 libjemalloc2 \
  libgsf-1-114 libfftw3-bin liborc-0.4-0 librsvg2-2 libcfitsio10 libimagequant0 libaom3 libheif1 \
  libx264-dev libx265-dev libnuma-dev libvpx7 libtheora0 libvorbis-dev \
  libspng0 libcgif0 && \
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
