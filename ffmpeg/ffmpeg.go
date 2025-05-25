package ffmpeg

// #cgo pkg-config: libavformat libavutil libavcodec libswscale
// #cgo CFLAGS: -std=c11
// #cgo LDFLAGS: -lm
// #include "ffmpeg.h"
import "C"
import (
	"github.com/cshum/vipsgen/pointer"
	"io"
	"math"
	"time"
	"unsafe"
)

const (
	readPacketFlag = 1
	seekPacketFlag = 2
	hasVideo       = 1
	hasAudio       = 2
)

// Metadata AV metadata
type Metadata struct {
	Orientation int     `json:"orientation"`
	Duration    int     `json:"duration,omitempty"`
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	Title       string  `json:"title,omitempty"`
	Artist      string  `json:"artist,omitempty"`
	FPS         float64 `json:"fps,omitempty"`
	HasVideo    bool    `json:"has_video"`
	HasAudio    bool    `json:"has_audio"`
}

// AVContext manages lifecycle of AV contexts and reader stream
type AVContext struct {
	opaque             unsafe.Pointer
	reader             io.Reader
	seeker             io.Seeker
	formatContext      *C.AVFormatContext
	stream             *C.AVStream
	codecContext       *C.AVCodecContext
	thumbContext       *C.ThumbContext
	selectedIndex      C.int
	selectedDuration   time.Duration
	frame              *C.AVFrame
	durationInFormat   bool
	orientation        int
	size               int64
	duration           time.Duration
	availableIndex     C.int
	availableDuration  time.Duration
	width, height      int
	title, artist      string
	hasVideo, hasAudio bool
	closed             bool
}

// LoadAVContext load and create AVContext from reader stream
func LoadAVContext(reader io.Reader, size int64) (*AVContext, error) {
	av := &AVContext{
		reader:        reader,
		size:          size,
		selectedIndex: -1,
	}
	if seeker, ok := reader.(io.Seeker); ok {
		av.seeker = seeker
	}
	flags := C.int(readPacketFlag)
	if av.seeker != nil {
		flags |= seekPacketFlag
	}
	if err := createFormatContext(av, flags); err != nil {
		return nil, err
	}
	if !av.hasVideo {
		return av, nil
	}
	return av, createDecoder(av)
}

// ProcessFrames triggers frame processing
// limit under max num of frames if maxFrames > 0
func (av *AVContext) ProcessFrames(maxFrames int) (err error) {
	if av.formatContext == nil || av.codecContext == nil {
		return ErrDecoderNotFound
	}
	if av.thumbContext == nil {
		return createThumbContext(av, C.int(maxFrames))
	}
	return
}

// SelectFrame triggers frame processing and select specific frame index
func (av *AVContext) SelectFrame(n int) (err error) {
	nn := C.int(n - 1)
	if av.thumbContext != nil && nn > av.availableIndex {
		nn = av.availableIndex
	}
	av.selectedIndex = nn
	return av.ProcessFrames(-1)
}

func (av *AVContext) positionToDuration(f float64) time.Duration {
	return time.Duration(float64(av.duration) * math.Max(math.Min(f, 1), 0))
}

func (av *AVContext) SelectPosition(f float64) (err error) {
	return av.SelectDuration(av.positionToDuration(f))
}

// SelectDuration seeks to keyframe before the specified duration
// then process frames to find precise duration
func (av *AVContext) SelectDuration(ts time.Duration) (err error) {
	if ts > 0 {
		av.selectedDuration = ts
		if err = av.SeekDuration(ts); err != nil {
			return
		}
		return av.ProcessFrames(-1)
	} else {
		return av.SelectFrame(1)
	}
}

// SeekPosition seeks to keyframe before specified position percentage between 0 and 1
// then process frames to find precise position
func (av *AVContext) SeekPosition(f float64) error {
	return av.SeekDuration(av.positionToDuration(f))
}

// SeekDuration seeks to keyframe before the  specified duration
func (av *AVContext) SeekDuration(ts time.Duration) error {
	if av.formatContext == nil || av.codecContext == nil {
		return ErrDecoderNotFound
	}
	return seekDuration(av, ts)
}

// Export frame to RGB or RGBA buffer
func (av *AVContext) Export(bands int) (buf []byte, err error) {
	if err = av.ProcessFrames(-1); err != nil {
		return
	}
	if bands < 3 || bands > 4 {
		bands = 4
	}
	if err = convertFrameToRGB(av, bands); err != nil {
		return
	}
	return exportBuffer(av, bands)
}

// Close AVContext objects
func (av *AVContext) Close() {
	closeAVContext(av)
}

// Metadata AV metadata
func (av *AVContext) Metadata() *Metadata {
	var fps float64
	if av.stream != nil {
		fps = float64(av.stream.r_frame_rate.num) / float64(av.stream.r_frame_rate.den)
	}
	return &Metadata{
		Orientation: av.orientation,
		Duration:    int(av.duration / time.Millisecond),
		Width:       av.width,
		Height:      av.height,
		Title:       av.title,
		Artist:      av.artist,
		FPS:         fps,
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
	metadata(av)
	duration(av)
	err := findStreams(av)
	if err != nil {
		C.free_format_context(av.formatContext)
		pointer.Unref(av.opaque)
	}
	return err
}

func metadata(av *AVContext) {
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

func seekDuration(av *AVContext, ts time.Duration) error {
	tts := C.int64_t(ts.Milliseconds()) * C.AV_TIME_BASE / 1000
	err := C.av_seek_frame(av.formatContext, C.int(-1), tts, C.AVSEEK_FLAG_BACKWARD)
	C.avcodec_flush_buffers(av.codecContext)
	if err < 0 {
		return avError(err)
	}
	return nil
}

func incrementDuration(av *AVContext, frame *C.AVFrame, i C.int) {
	av.availableIndex = i
	if frame.pts != C.AV_NOPTS_VALUE {
		ptsToNano := C.int64_t(1000000000 * av.stream.time_base.num / av.stream.time_base.den)
		newDuration := time.Duration(frame.pts * ptsToNano)
		av.availableDuration = newDuration

		if !av.durationInFormat && newDuration > av.duration {
			av.duration = newDuration
		}
	}
}

func populateFrames(av *AVContext, frames <-chan *C.AVFrame) <-chan struct{} {
	done := make(chan struct{})
	var isSelected = av.selectedIndex > -1
	go func() {
		var n C.int
		if !isSelected {
			for frame := range frames {
				C.populate_histogram(av.thumbContext, n, frame)
				n++
			}
		} else {
			for frame := range frames {
				C.populate_frame(av.thumbContext, n, frame)
				n++
			}
		}
		av.thumbContext.n = n
		close(done)
	}()
	return done
}

func createThumbContext(av *AVContext, maxFrames C.int) error {
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
	n := av.thumbContext.max_frames
	if maxFrames > 0 && n > maxFrames {
		n = maxFrames
	}
	if av.selectedIndex > -1 && n > av.selectedIndex+1 {
		n = av.selectedIndex + 1
	}
	if av.selectedDuration > 0 && av.selectedIndex < 0 {
		av.selectedIndex = 0
	}
	frames := make(chan *C.AVFrame, n)
	done := populateFrames(av, frames)
	frames <- frame
	if pkt.buf != nil {
		C.av_packet_unref(&pkt)
	}
	return populateThumbContext(av, frames, n, done)
}

func populateThumbContext(av *AVContext, frames chan *C.AVFrame, n C.int, done <-chan struct{}) error {
	pkt := C.create_packet()
	var frame *C.AVFrame
	var err C.int
	for i := C.int(1); i < n; i++ {
		err = C.obtain_next_frame(av.formatContext, av.codecContext, av.stream.index, &pkt, &frame)
		if err < 0 {
			break
		}
		incrementDuration(av, frame, i)
		frames <- frame
		frame = nil
		if av.selectedDuration > 0 {
			if av.availableDuration <= av.selectedDuration {
				av.selectedIndex = i
			} else {
				break
			}
		}
	}
	if av.selectedIndex > av.availableIndex {
		av.selectedIndex = av.availableIndex
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
	if av.selectedIndex < 0 {
		av.selectedIndex = C.find_best_frame_index(av.thumbContext)
	}
	return nil
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
