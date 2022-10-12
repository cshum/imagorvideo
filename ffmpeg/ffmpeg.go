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
	HasVideo    bool   `json:"has_video"`
	HasAudio    bool   `json:"has_audio"`
	HasAlpha    bool   `json:"has_alpha"`
}

type AVContext struct {
	context          context.Context
	opaque           unsafe.Pointer
	reader           io.Reader
	seeker           io.Seeker
	formatContext    *C.AVFormatContext
	stream           *C.AVStream
	codecContext     *C.AVCodecContext
	thumbContext     *C.ThumbContext
	frame            *C.AVFrame
	outputFrame      *C.AVFrame
	durationInFormat bool

	orientation        int
	size               int64
	duration           time.Duration
	width, height      int
	title, artist      string
	hasVideo, hasAudio bool
	hasAlpha           bool
	closed             bool
}

func LoadAVContext(ctx context.Context, reader io.Reader, size int64) (*AVContext, error) {
	av := &AVContext{
		context: ctx,
		reader:  reader,
		size:    size,
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
	if err := createThumbContext(av); err != nil {
		return av, err
	}
	if err := convertFrameToRGB(av); err != nil {
		return av, err
	}
	return av, nil
}

func closeAVContext(av *AVContext) {
	if av.closed {
		return
	}
	if av.outputFrame != nil {
		C.av_frame_free(&av.outputFrame)
	}
	if av.outputFrame != nil {
		C.av_frame_free(&av.outputFrame)
	}
	if av.thumbContext != nil {
		C.free_thumb_context(av.thumbContext)
		av.frame = nil
	}
	if av.codecContext != nil {
		C.avcodec_free_context(&av.codecContext)
	}
	if av.formatContext != nil {
		C.free_format_context(av.formatContext)
	}
	pointer.Unref(av.opaque)
}

func (av *AVContext) Export() (buf []byte, err error) {
	return exportBuffer(av)
}

func (av *AVContext) Close() {
	closeAVContext(av)
}

func (av *AVContext) Metadata() *Metadata {
	return &Metadata{
		Orientation: av.orientation,
		Duration:    int(av.duration / time.Millisecond),
		Width:       av.width,
		Height:      av.height,
		Title:       av.title,
		Artist:      av.artist,
		HasVideo:    av.hasVideo,
		HasAudio:    av.hasAudio,
		HasAlpha:    av.hasAlpha,
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

func incrementDuration(av *AVContext, frame *C.AVFrame) {
	if !av.durationInFormat && frame.pts != C.AV_NOPTS_VALUE {
		ptsToNano := C.int64_t(1000000000 * av.stream.time_base.num / av.stream.time_base.den)
		newDuration := time.Duration(frame.pts * ptsToNano)
		if newDuration > av.duration {
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
		incrementDuration(av, frame)
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
		incrementDuration(av, frame)
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
	av.frame = C.process_frames(av.thumbContext)
	if av.frame == nil {
		return ErrNoMem
	}
	return nil
}

func convertFrameToRGB(av *AVContext) error {
	av.outputFrame = C.convert_frame_to_rgb(av.frame, av.thumbContext.alpha)
	if av.outputFrame == nil {
		return ErrNoMem
	}
	av.hasAlpha = av.thumbContext.alpha != 0
	return nil
}

func exportBuffer(av *AVContext) ([]byte, error) {
	if av.outputFrame == nil {
		return nil, ErrInvalidData
	}
	size := av.height * av.width
	if av.hasAlpha {
		size *= 4
	} else {
		size *= 3
	}
	buf := C.GoBytes(unsafe.Pointer(av.outputFrame.data[0]), C.int(size))
	return buf, nil
}
