package imagorvideo

import (
	"flag"
	"github.com/cshum/imagor"
	"go.uber.org/zap"
)

func Config(fs *flag.FlagSet, cb func() (*zap.Logger, bool)) imagor.Option {
	var (
		ffmpegFallbackImage = fs.String("ffmpeg-fallback-image", "",
			"FFmpeg fallback image on processing error. Supports image path enabled by loaders or storages")

		logger, isDebug = cb()
	)
	return imagor.WithProcessors(
		NewProcessor(
			WithFallbackImage(*ffmpegFallbackImage),
			WithLogger(logger),
			WithDebug(isDebug),
		),
	)
}
