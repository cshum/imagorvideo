package ffmpeg

// #cgo pkg-config: libavformat libavutil libavcodec libswscale
// #cgo CFLAGS: -std=c11
// #cgo LDFLAGS: -lm
// #include "ffmpeg.h"
import "C"
import (
	"context"
	"github.com/cshum/imagor/vips/pointer"
	"io"
	"time"
	"unsafe"
)

const (
	readPacketFlag = 1
	seekPacketFlag = 2
	interruptFlag  = 3
	hasVideo       = 1
	hasAudio       = 2
)

type Metadata struct {
	Orientation int    `json:"orientation"`
	Duration    int    `json:"duration,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Title       string `json:"title,omitempty"`
	Artist      string `json:"artist,omitempty"`
	FPS         int    `json:"fps,omitempty"`
	HasVideo    bool   `json:"has_video"`
	HasAudio    bool   `json:"has_audio"`
}

type AVContext struct {
	context            context.Context
	opaque             unsafe.Pointer
	reader             io.Reader
	seeker             io.Seeker
	formatContext      *C.AVFormatContext
	stream             *C.AVStream
	codecContext       *C.AVCodecContext
	thumbContext       *C.ThumbContext
	selectedIndex      C.int
	frame              *C.AVFrame
	durationInFormat   bool
	orientation        int
	size               int64
	duration           time.Duration
	indexAt            C.int
	durationAt         time.Duration
	width, height      int
	title, artist      string
	hasVideo, hasAudio bool
	closed             bool
}

func LoadAVContext(ctx context.Context, reader io.Reader, size int64) (*AVContext, error) {
	av := &AVContext{
		context:       ctx,
		reader:        reader,
		size:          size,
		selectedIndex: -1,
	}
	if seeker, ok := reader.(io.Seeker); ok {
		av.seeker = seeker
	}
	flags := C.int(readPacketFlag | interruptFlag)
	if av.seeker != nil {
		flags |= seekPacketFlag
	}
	if err := createFormatContext(av, flags); err != nil {
		return nil, err
	}
	if !av.hasVideo {
		return av, nil
	}
	if err := createDecoder(av); err != nil {
		return av, err
	}
	return av, nil
}

//
//func (av *AVContext) SelectFrame(n int) (err error) {
//	if av.thumbContext == nil || av.thumbContext.max_frames {
//		err = ErrInvalidData
//		return
//	}
//}

func (av *AVContext) Export(bands int) (buf []byte, err error) {
	if bands < 3 || bands > 4 {
		bands = 3
	}
	if err = createThumbContext(av); err != nil {
		return
	}
	if av.selectedIndex < 0 {
		findBestFrameIndex(av)
	}
	if err = convertFrameToRGB(av, bands); err != nil {
		return
	}
	return exportBuffer(av, bands)
}

func (av *AVContext) Close() {
	closeAVContext(av)
}

func (av *AVContext) Metadata() *Metadata {
	var fps float64
	if av.durationAt > 0 {
		fps = float64(av.indexAt) * float64(time.Second) / float64(av.durationAt)
	}
	return &Metadata{
		Orientation: av.orientation,
		Duration:    int(av.duration / time.Millisecond),
		Width:       av.width,
		Height:      av.height,
		Title:       av.title,
		Artist:      av.artist,
		FPS:         int(fps),
		HasVideo:    av.hasVideo,
		HasAudio:    av.hasAudio,
	}
}

func closeAVContext(av *AVContext) {
	if !av.closed {
		if av.frame != nil {
			C.av_frame_free(&av.frame)
		}
		if av.thumbContext != nil {
			C.free_thumb_context(av.thumbContext)
		}
		if av.codecContext != nil {
			C.avcodec_free_context(&av.codecContext)
		}
		if av.formatContext != nil {
			C.free_format_context(av.formatContext)
		}
		pointer.Unref(av.opaque)
		av.closed = true
	}
}

func createFormatContext(av *AVContext, callbackFlags C.int) error {
	intErr := C.allocate_format_context(&av.formatContext)
	if intErr < 0 {
		return avError(intErr)
	}
	av.opaque = pointer.Save(av)
	intErr = C.create_format_context(av.formatContext, av.opaque, callbackFlags)
	if intErr < 0 {
		pointer.Unref(av.opaque)
		return avError(intErr)
	}
	metaData(av)
	duration(av)
	err := findStreams(av)
	if err != nil {
		C.free_format_context(av.formatContext)
		pointer.Unref(av.opaque)
	}
	return err
}

func metaData(av *AVContext) {
	var artist, title *C.char
	C.get_metadata(av.formatContext, &artist, &title)
	av.artist = C.GoString(artist)
	av.title = C.GoString(title)
}

func duration(av *AVContext) {
	if av.formatContext.duration > 0 {
		av.durationInFormat = true
		av.duration = time.Duration(1000 * av.formatContext.duration)
	}
}

func findStreams(av *AVContext) error {
	var orientation C.int
	err := C.find_streams(av.formatContext, &av.stream, &orientation)
	if err < 0 {
		return avError(err)
	}
	av.hasVideo = err&hasVideo != 0
	av.hasAudio = err&hasAudio != 0
	if av.hasVideo {
		av.width = int(av.stream.codecpar.width)
		av.height = int(av.stream.codecpar.height)
		av.orientation = int(orientation)
	}
	return nil
}

func createDecoder(av *AVContext) error {
	err := C.create_codec_context(av.stream, &av.codecContext)
	if err < 0 {
		return avError(err)
	}
	return nil
}

func incrementDuration(av *AVContext, frame *C.AVFrame, i C.int) {
	av.indexAt = i
	if frame.pts != C.AV_NOPTS_VALUE {
		ptsToNano := C.int64_t(1000000000 * av.stream.time_base.num / av.stream.time_base.den)
		newDuration := time.Duration(frame.pts * ptsToNano)
		av.durationAt = newDuration
		if !av.durationInFormat && newDuration > av.duration {
			av.duration = newDuration
		}
	}
}

func populateHistogram(av *AVContext, frames <-chan *C.AVFrame) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		var n C.int
		for frame := range frames {
			C.populate_histogram(av.thumbContext, n, frame)
			n++
		}
		av.thumbContext.n = n
		done <- struct{}{}
		close(done)
	}()
	return done
}

func createThumbContext(av *AVContext) error {
	pkt := C.create_packet()
	var frame *C.AVFrame
	err := C.obtain_next_frame(av.formatContext, av.codecContext, av.stream.index, &pkt, &frame)
	if err >= 0 {
		incrementDuration(av, frame, 0)
		av.thumbContext = C.create_thumb_context(av.stream, frame)
		if av.thumbContext == nil {
			err = C.int(ErrNoMem)
		}
	}
	if err < 0 {
		if pkt.buf != nil {
			C.av_packet_unref(&pkt)
		}
		if frame != nil {
			C.av_frame_free(&frame)
		}
		return avError(err)
	}
	frames := make(chan *C.AVFrame, av.thumbContext.max_frames)
	done := populateHistogram(av, frames)
	frames <- frame
	if pkt.buf != nil {
		C.av_packet_unref(&pkt)
	}
	return populateThumbContext(av, frames, done)
}

func populateThumbContext(av *AVContext, frames chan *C.AVFrame, done <-chan struct{}) error {
	pkt := C.create_packet()
	var frame *C.AVFrame
	var err C.int
	for i := C.int(1); i < av.thumbContext.max_frames; i++ {
		err = C.obtain_next_frame(av.formatContext, av.codecContext, av.stream.index, &pkt, &frame)
		if err < 0 {
			break
		}
		incrementDuration(av, frame, i)
		frames <- frame
		frame = nil
	}
	close(frames)
	if pkt.buf != nil {
		C.av_packet_unref(&pkt)
	}
	if frame != nil {
		C.av_frame_free(&frame)
	}
	<-done
	if err != 0 && err != C.int(ErrEOF) {
		return avError(err)
	}
	return nil
}

func findBestFrameIndex(av *AVContext) {
	av.selectedIndex = C.find_best_frame_index(av.thumbContext)
}

func convertFrameToRGB(av *AVContext, bands int) error {
	var alpha int
	if bands == 4 {
		alpha = 1
	}
	av.frame = C.convert_frame_to_rgb(
		C.select_frame(av.thumbContext, av.selectedIndex), C.int(alpha))
	if av.frame == nil {
		return ErrNoMem
	}
	return nil
}

func exportBuffer(av *AVContext, bands int) ([]byte, error) {
	if av.frame == nil {
		return nil, ErrInvalidData
	}
	size := av.height * av.width * bands
	buf := C.GoBytes(unsafe.Pointer(av.frame.data[0]), C.int(size))
	return buf, nil
}
