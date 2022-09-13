package imagorvideo

import (
	"context"
	"github.com/cshum/imagor"
	"github.com/cshum/imagor/imagorpath"
	"github.com/cshum/imagorvideo/ffmpeg"
	"github.com/gabriel-vasile/mimetype"
	"go.uber.org/zap"
	"io"
	"os"
	"strings"
)

type Processor struct {
	Logger        *zap.Logger
	Debug         bool
	FallbackImage string
}

func NewProcessor(options ...Option) *Processor {
	p := &Processor{
		Logger: zap.NewNop(),
	}
	for _, option := range options {
		option(p)
	}
	return p
}

func (p *Processor) Startup(_ context.Context) error {
	ffmpeg.SetLogging(func(level ffmpeg.AVLogLevel, message string) {
		message = strings.TrimSuffix(message, "\n")
		switch level {
		case ffmpeg.AVLogTrace, ffmpeg.AVLogDebug, ffmpeg.AVLogVerbose:
			p.Logger.Debug("ffmpeg", zap.String("log", message))
		case ffmpeg.AVLogInfo:
			p.Logger.Info("ffmpeg", zap.String("log", message))
		case ffmpeg.AVLogWarning, ffmpeg.AVLogError, ffmpeg.AVLogFatal, ffmpeg.AVLogPanic:
			p.Logger.Warn("ffmpeg", zap.String("log", message))
		}
	})
	if p.Debug {
		ffmpeg.SetFFmpegLogLevel(ffmpeg.AVLogDebug)
	} else {
		ffmpeg.SetFFmpegLogLevel(ffmpeg.AVLogError)
	}
	return nil
}

func (p *Processor) Shutdown(_ context.Context) error {
	return nil
}

func (p *Processor) Process(ctx context.Context, in *imagor.Blob, params imagorpath.Params, load imagor.LoadFunc) (out *imagor.Blob, err error) {
	defer func() {
		if err == nil || out != nil {
			return
		}
		if _, ok := err.(imagor.ErrForward); ok {
			return
		}
		// fallback image on error
		out = imagor.NewBlobFromBytes(transPixel)
		if p.FallbackImage != "" {
			if o, e := load(p.FallbackImage); e == nil {
				out = o
			}
		}
	}()
	var filters imagorpath.Filters
	var mime = mimetype.Detect(in.Sniff()).String()
	if !strings.HasPrefix(mime, "video/") &&
		!strings.HasPrefix(mime, "audio/") {
		// forward identical for non video nor audio
		err = imagor.ErrForward{Params: params}
		out = in
		return
	}
	var reader io.ReadCloser
	var size int64
	switch mime {
	case "video/webm", "video/x-matroska":
		// media types that does not require seek
		if reader, size, err = in.NewReader(); err != nil {
			return
		}
	default:
		if reader, size, err = in.NewReadSeeker(); err != nil {
			// write to temp file if read seeker not available
			if reader, _, err = in.NewReader(); err != nil {
				return
			}
			var file *os.File
			if file, err = os.CreateTemp("", "imagor-"); err != nil {
				return
			}
			var filename = file.Name()
			defer func() {
				_ = os.Remove(filename)
				p.Logger.Debug("cleanup", zap.String("file", filename))
			}()
			if size, err = io.Copy(file, reader); err != nil {
				return
			}
			p.Logger.Debug("temp",
				zap.String("file", filename),
				zap.Int64("size", size))
			_ = file.Close()
			if reader, err = os.Open(filename); err != nil {
				return
			}
		}
	}
	av, err := ffmpeg.LoadAVContext(ctx, reader, size)
	if err != nil {
		return
	}
	defer av.Close()
	meta := av.Metadata()
	if params.Meta {
		out = imagor.NewBlobFromJsonMarshal(meta)
		return
	}
	switch meta.Orientation {
	case 3:
		filters = append(filters, imagorpath.Filter{Name: "rotate", Args: "180"})
	case 6:
		filters = append(filters, imagorpath.Filter{Name: "rotate", Args: "270"})
	case 8:
		filters = append(filters, imagorpath.Filter{Name: "rotate", Args: "90"})
	}
	buf, err := av.Export()
	if err != nil || len(buf) == 0 {
		if err == nil {
			err = imagor.ErrUnsupportedFormat
		}
		return
	}
	bands := 3
	if meta.HasAlpha {
		bands = 4
	}
	out = imagor.NewBlobFromMemory(buf, meta.Width, meta.Height, bands)
	if len(filters) > 0 {
		params.Filters = append(filters, params.Filters...)
		params.Path = imagorpath.GeneratePath(params)
	}
	err = imagor.ErrForward{Params: params}
	return
}

var transPixel = []byte("\x47\x49\x46\x38\x39\x61\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\x00\x00\x00\x21\xF9\x04\x01\x00\x00\x00\x00\x2C\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02\x44\x01\x00\x3B")
