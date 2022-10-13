package imagorvideo

import (
	"context"
	"github.com/cshum/imagor"
	"github.com/cshum/imagor/imagorpath"
	"github.com/cshum/imagorvideo/ffmpeg"
	"github.com/gabriel-vasile/mimetype"
	"go.uber.org/zap"
	"io"
	"strconv"
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
		err = imagor.NewError(err.Error(), 406)
		// fallback image on error
		out = imagor.NewBlobFromBytes(transPixel)
		if p.FallbackImage != "" {
			if o, e := load(p.FallbackImage); e == nil {
				out = o
			}
		}
	}()
	var filters imagorpath.Filters
	var mime = mimetype.Detect(in.Sniff())
	if typ := mime.String(); !strings.HasPrefix(typ, "video/") &&
		!strings.HasPrefix(typ, "audio/") {
		// forward identical for non video nor audio
		err = imagor.ErrForward{Params: params}
		out = in
		return
	}
	var r io.ReadCloser
	var rs io.ReadSeekCloser
	var size = in.Size()
	if size > 0 {
		switch mime.String() {
		case "video/webm", "video/x-matroska":
			// media types that does not require seek
			if r, _, err = in.NewReader(); err != nil {
				return
			}
		}
	}
	if r == nil {
		if rs, size, err = in.NewReadSeeker(); err != nil {
			return
		}
		r = rs
	}
	defer func() {
		_ = r.Close()
	}()
	if size <= 0 && rs != nil {
		// size must be known
		if size, err = rs.Seek(0, io.SeekEnd); err != nil {
			return
		}
		if _, err = rs.Seek(0, io.SeekStart); err != nil {
			return
		}
	}
	av, err := ffmpeg.LoadAVContext(ctx, r, size)
	if err != nil {
		return
	}
	defer av.Close()
	bands := 3
	for _, filter := range params.Filters {
		switch filter.Name {
		case "format":
			switch strings.ToLower(filter.Args) {
			case "webp", "png", "gif":
				switch mime.Extension() {
				case ".webm", ".flv", ".mov", ".avi":
					bands = 4
				}
			}
		case "process_frames":
			if err = av.ProcessFrames(); err != nil {
				return
			}
		case "frame":
			n, _ := strconv.Atoi(filter.Args)
			if err = av.SelectFrame(n); err != nil {
				return
			}
		}
	}
	meta := av.Metadata()
	if params.Meta {
		out = imagor.NewBlobFromJsonMarshal(Metadata{
			Format:      strings.TrimPrefix(mime.Extension(), "."),
			ContentType: mime.String(),
			Metadata:    meta,
		})
		return
	}
	switch meta.Orientation {
	case 3:
		filters = append(filters, imagorpath.Filter{Name: "orient", Args: "180"})
	case 6:
		filters = append(filters, imagorpath.Filter{Name: "orient", Args: "270"})
	case 8:
		filters = append(filters, imagorpath.Filter{Name: "orient", Args: "90"})
	}
	buf, err := av.Export(bands)
	if err != nil || len(buf) == 0 {
		if err == nil {
			err = imagor.ErrUnsupportedFormat
		}
		return
	}
	out = imagor.NewBlobFromMemory(buf, meta.Width, meta.Height, bands)

	if len(filters) > 0 {
		params.Filters = append(params.Filters, filters...)
		params.Path = imagorpath.GeneratePath(params)
	}
	err = imagor.ErrForward{Params: params}
	return
}

type Metadata struct {
	Format      string `json:"format"`
	ContentType string `json:"content_type"`
	*ffmpeg.Metadata
}

var transPixel = []byte("\x47\x49\x46\x38\x39\x61\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\x00\x00\x00\x21\xF9\x04\x01\x00\x00\x00\x00\x2C\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02\x44\x01\x00\x3B")
