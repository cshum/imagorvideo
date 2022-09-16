package ffmpeg

// #cgo pkg-config: libavformat libavutil libavcodec libswscale
// #cgo CFLAGS: -std=c11
// #cgo LDFLAGS: -lm
// #include "ffmpeg.h"
import "C"
import (
	"context"
	"github.com/cshum/imagorvideo/ffmpeg/pointer"
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
	Orientation int           `json:"orientation"`
	Duration    time.Duration `json:"duration,omitempty"`
	Width       int           `json:"width,omitempty"`
	Height      int           `json:"height,omitempty"`
	Title       string        `json:"title,omitempty"`
	Artist      string        `json:"artist,omitempty"`
	HasVideo    bool          `json:"has_video"`
	HasAudio    bool          `json:"has_audio"`
	HasAlpha    bool          `json:"has_alpha"`
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
	durationInFormat bool

	orientation        int
	size               int64
	duration           time.Duration
	width, height      int
	title, artist      string
	hasVideo, hasAudio bool
	hasFrame, hasAlpha bool
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
	err := createFormatContext(av, flags)
	if err != nil {
		return nil, err
	}
	if !av.hasVideo {
		return av, nil
	}
	if err = createDecoder(av); err == ErrTooBig || err == ErrDecoderNotFound {
		return av, err
	}
	return av, nil
}

func (av *AVContext) Export() (buf []byte, err error) {
	return exportBuffer(av)
}

func (av *AVContext) Close() {
	if av.hasFrame {
		C.av_frame_free(&av.frame)
	}
	freeFormatContext(av)
}

func (av *AVContext) Metadata() *Metadata {
	return &Metadata{
		Orientation: av.orientation,
		Duration:    av.duration,
		Width:       av.width,
		Height:      av.height,
		Title:       av.title,
		Artist:      av.artist,
		HasVideo:    av.hasVideo,
		HasAudio:    av.hasAudio,
		HasAlpha:    av.hasAlpha,
	}
}

func freeFormatContext(av *AVContext) {
	C.free_format_context(av.formatContext)
	pointer.Unref(av.opaque)
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
		freeFormatContext(av)
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

func fullDuration(av *AVContext) error {
	if av.durationInFormat {
		return nil
	}
	newDuration := time.Duration(C.find_duration(av.formatContext))
	if newDuration < 0 {
		return avError(newDuration)
	}
	if newDuration > av.duration {
		av.duration = newDuration
	}
	return nil
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
	defer C.avcodec_free_context(&av.codecContext)
	return createThumbContext(av)
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
	defer C.free_thumb_context(av.thumbContext)
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
	return convertFrameToRGB(av)
}

func convertFrameToRGB(av *AVContext) error {
	outputFrame := C.convert_frame_to_rgb(C.process_frames(av.thumbContext), av.thumbContext.alpha)
	if outputFrame == nil {
		return ErrNoMem
	}
	av.frame = outputFrame
	av.hasFrame = true
	av.hasAlpha = av.thumbContext.alpha != 0
	return nil
}

func exportBuffer(av *AVContext) ([]byte, error) {
	if !av.hasFrame {
		return nil, ErrInvalidData
	}
	size := av.height * av.width
	if av.hasAlpha {
		size *= 4
	} else {
		size *= 3
	}
	buf := C.GoBytes(unsafe.Pointer(av.frame.data[0]), C.int(size))
	return buf, nil
}
