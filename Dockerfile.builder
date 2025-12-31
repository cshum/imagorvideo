ARG GOLANG_VERSION=1.25.5
ARG FFMPEG_VERSION=7.1.1
ARG VIPS_VERSION=8.18.0

FROM golang:${GOLANG_VERSION}-trixie AS cache-builder

ARG FFMPEG_VERSION
ARG VIPS_VERSION
ARG TARGETARCH

ENV PKG_CONFIG_PATH=/usr/local/lib/pkgconfig
ENV MAKEFLAGS="-j8"

# Install libvips + FFmpeg + required libraries
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
  rm -rf /usr/local/lib/*.la && \
  rm -rf /tmp/*

# This cache image provides Go + compiled libvips + FFmpeg libraries
LABEL maintainer="imagorvideo"
