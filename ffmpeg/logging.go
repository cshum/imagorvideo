package ffmpeg

// #include "ffmpeg.h"
import "C"

// AVLogLevel defines the ffmpeg threshold for dumping information to stderr.
type AVLogLevel int

// Possible values for AVLogLevel.
const (
	AVLogQuiet AVLogLevel = (iota - 1) * 8
	AVLogPanic
	AVLogFatal
	AVLogError
	AVLogWarning
	AVLogInfo
	AVLogVerbose
	AVLogDebug
	AVLogTrace
)

func logLevel() AVLogLevel {
	return AVLogLevel(C.av_log_get_level())
}

// SetFFmpegLogLevel allows you to change the log level from the default (AVLogInfo).
func SetFFmpegLogLevel(logLevel AVLogLevel) {
	C.av_log_set_level(C.int(logLevel))
}
