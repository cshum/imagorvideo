# imagorvideo

[![Test Status](https://github.com/cshum/imagorvideo/workflows/test/badge.svg)](https://github.com/cshum/imagorvideo/actions/workflows/test.yml)
[![Coverage Status](https://img.shields.io/coveralls/github/cshum/imagorvideo)](https://coveralls.io/github/cshum/imagorvideo?branch=master)
[![Docker Hub](https://img.shields.io/badge/docker-shumc/imagorvideo-blue.svg)](https://hub.docker.com/r/shumc/imagorvideo/)
[![GitHub Container Registry](https://ghcr-badge.herokuapp.com/cshum/imagorvideo/latest_tag?trim=major&label=ghcr.io&ignore=next,master&color=%23007ec6)](https://github.com/cshum/imagorvideo/pkgs/container/imagorvideo)


imagorvideo is a new initiative that brings video thumbnail capability through ffmpeg, built on the foundations of [imagor](https://github.com/cshum/imagor) - a fast, Docker-ready image processing server written in Go with libvips.

Imagorvideo uses ffmpeg C bindings that extracts image thumbnail from video by attempting to select the best frame, then forwards to libvips to perform all existing image operations supported by imagor.

imagorvideo uses reader stream for mkv and webm video types. For other video types that requires seeking from a non seek-able source such as HTTP or S3, it simulates seek using memory buffer or temp file, by having the whole file to be fully loaded to perform seek.

This also aims to be a reference project demonstrating imagor extension.


### Quick Start

```bash
docker run -p 8000:8000 shumc/imagorvideo -imagor-unsafe
```

Original:
```
https://test-videos.co.uk/vids/bigbuckbunny/mkv/1080/Big_Buck_Bunny_1080_10s_5MB.mkv
```

Result:
```
http://localhost:8000/unsafe/fit-in/300x200/filters:label(imagorvideo,-10,10,20,yellow):fill(yellow)/https://test-videos.co.uk/vids/bigbuckbunny/mkv/1080/Big_Buck_Bunny_1080_10s_5MB.mkv
```
<img src="https://raw.githubusercontent.com/cshum/imagorvideo/master/testdata/demo.png" width="200" />

Check out [imagor](https://github.com/cshum/imagor#image-endpoint) for all existing image operations supported.

### Configuration

Config options specific to imagorvideo. Please refer to [imagor](https://github.com/cshum/imagor#configuration) for all existing options supported.

```
  -ffmpeg-fallback-image string
        FFmpeg fallback image on processing error. Supports image path enabled by loaders or storages
```


