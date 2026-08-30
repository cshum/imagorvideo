package imagorvideo

import (
	"flag"
	"github.com/cshum/imagor"
	"go.uber.org/zap"
)

// Config imagorvideo config.Option
func Config(fs *flag.FlagSet, cb func() (*zap.Logger, bool)) imagor.Option {
	var (
		ffmpegFallbackImage = fs.String("ffmpeg-fallback-image", "",
			"FFmpeg fallback image on processing error. Supports image path enabled by loaders or storages")
		ffmpegMaxAnimationFrames = fs.Int("ffmpeg-max-animation-frames", defaultMaxAnimationFrames,
			"FFmpeg maximum number of frames decoded by the gif filter")

		logger, isDebug = cb()
	)
	return imagor.WithProcessors(
		NewProcessor(
			WithFallbackImage(*ffmpegFallbackImage),
			WithMaxAnimationFrames(*ffmpegMaxAnimationFrames),
			WithLogger(logger),
			WithDebug(isDebug),
		),
	)
}
