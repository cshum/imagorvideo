# imagorvideo

[![Test Status](https://github.com/cshum/imagorvideo/workflows/test/badge.svg)](https://github.com/cshum/imagorvideo/actions/workflows/test.yml)
[![Coverage Status](https://img.shields.io/coveralls/github/cshum/imagorvideo)](https://coveralls.io/github/cshum/imagorvideo?branch=master)
[![Docker Hub](https://img.shields.io/badge/docker-shumc/imagorvideo-blue.svg)](https://hub.docker.com/r/shumc/imagorvideo/)
[![GitHub Container Registry](https://ghcr-badge.herokuapp.com/cshum/imagorvideo/latest_tag?trim=major&label=ghcr.io&ignore=next,master&color=%23007ec6)](https://github.com/cshum/imagorvideo/pkgs/container/imagorvideo)

imagorvideo is a new initiative that brings video thumbnail capability through ffmpeg, built on the foundations of [imagor](https://github.com/cshum/imagor) - a fast, Docker-ready image processing server written in Go with libvips.

imagorvideo uses ffmpeg C bindings that extracts image thumbnail from video, by selecting the best frame from a RMSE histogram. It then forwards to libvips to perform image [cropping, resizing](https://github.com/cshum/imagor#image-endpoint) and [filters](https://github.com/cshum/imagor#filters) provided by imagor.

imagorvideo integrates ffmpeg read and seek I/O callbacks with imagor [loader, storage and result storage](https://github.com/cshum/imagor#loader-storage-and-result-storage), which supports HTTP(s), File System, AWS S3 and Google Cloud Storage out of box. For non seek-able source such as HTTP and S3, imagor simulates seek using memory or temp file buffer.

This also aims to be a reference project demonstrating imagor extension.

### Quick Start

```bash
docker run -p 8000:8000 shumc/imagorvideo -imagor-unsafe
```

Original:
```
http://commondatastorage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4
```

Result:
```
http://localhost:8000/unsafe/300x0/7x7/filters:label(imagorvideo,-10,-7,15,yellow):fill(yellow)/http://commondatastorage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4
http://localhost:8000/unsafe/300x0/0x0:0x14/filters:frame(1m59s):fill(yellow):label(imagorvideo,center,bottom,12,black,20)/http://commondatastorage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4
http://localhost:8000/unsafe/300x0/7x7/filters:frame(0.6):label(imagorvideo,10,-7,15,yellow):fill(yellow)/http://commondatastorage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4
```

<img src="https://raw.githubusercontent.com/cshum/imagorvideo/master/testdata/demo.jpg" height="150" /> <img src="https://raw.githubusercontent.com/cshum/imagorvideo/master/testdata/demo2.jpg" height="150" /> <img src="https://raw.githubusercontent.com/cshum/imagorvideo/master/testdata/demo3.jpg" height="150" /> 

imagorvideo works by streaming out a limited number of frame data, looping through and calculating the histogram of each frame. It then choose the best frame based on Root Mean Square Error (RMSE). This allow skipping the black frames that usually occur at the beginning of videos. 

imagorvideo then converts the selected frame to RGB image data, forwards to the imagor libvips processor, which has always been best at image processing with tons of features. Check out [imagor documentations](https://github.com/cshum/imagor#image-endpoint) for image operations supported.

### Filters

imagorvideo supports the following filters, which can be used in conjunction with [imagor filters](https://github.com/cshum/imagor#filters):

- `frame(n)` specify the position or time duration for imaging, which skips the automatic best frame selection:
  - Float between `0.0` and `1.0` position index of the video. Example `frame(0.5)`, `frame(1.0)`
  - Time duration of the elapsed time since the start of video. Example `frame(5m1s)`, `frame(200s)`
- `seek(n)` seeks to the approximate position or time duration, then perform automatic best frame selection around that point:
  - Float between `0.0` and `1.0` position index of the video. Example `seek(0.5)`
  - Time duration of the elapsed time since the start of video. Example `seek(5m1s)`, `seek(200s)`
- `max_frames(n)` restrict the maximum number of frames allocated for image selection. The smaller the number, the faster the processing time.

#### `frame(n)` vs `seek(n)`

There are differences you may want to choose one over the other.
`frame(n)` gives you the precise time frame specified. However, precise may not be the best in some circumstances:
```
http://commondatastorage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4
```
Retrieving the frame at 5 minutes elapsed time of this video:
```
http://localhost:8000/unsafe/filters:frame(5m)/http://commondatastorage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4
```
It results a complete black frame. 

![black](https://raw.githubusercontent.com/cshum/imagorvideo/master/testdata/black.jpg)

This is where `seek(n)` comes handy. It seeks to the key frame before the 5 minutes elapsed time, then perform best frame selection starting from that point using Root Mean Square Error (RMSE) histogram.
The result is a reasonable image that sits close to the specified time:

```
http://localhost:8000/unsafe/filters:seek(5m)/http://commondatastorage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4
```
![seek 5m](https://raw.githubusercontent.com/cshum/imagorvideo/master/testdata/seek5m.jpg)

### Metadata

imagorvideo provides metadata endpoint that extracts video metadata, including dimension, duration and FPS data. It processes header only, without extracting the frame data for better processing speed.

To use the metadata endpoint, add `/meta` right after the URL signature hash before the image operations:

```
http://localhost:8000/unsafe/meta/https://test-videos.co.uk/vids/bigbuckbunny/mp4/h264/1080/Big_Buck_Bunny_1080_10s_30MB.mp4
```

```jsonc
{
  "format": "mp4",
  "content_type": "video/mp4",
  "orientation": 1,
  "duration": 10000,
  "width": 1920,
  "height": 1080,
  "title": "Big Buck Bunny, Sunflower version",
  "artist": "Blender Foundation 2008, Janus Bager Kristensen 2013",
  "fps": 30,
  "has_video": true,
  "has_audio": false
}
```

### Configuration

Configuration options specific to imagorvideo. Please see [imagor configuration](https://github.com/cshum/imagor#configuration) for all existing options available.

```
  -ffmpeg-fallback-image string
        FFmpeg fallback image on processing error. Supports image path enabled by loaders or storages
```


