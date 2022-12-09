package ffmpeg

// #include "ffmpeg.h"
// #include "logging.h"
import "C"
import "sync"

// AVLogLevel defines the ffmpeg threshold for dumping information to stderr.
type AVLogLevel int

// AVLogLevel enum
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

var (
	currentLoggingHandlerFunction = noopLoggingHandler
	currentLoggingVerbosity       AVLogLevel
	onceLogging                   sync.Once
)

// SetFFmpegLogLevel allows you to change the log level from the default (AVLogInfo).
func SetFFmpegLogLevel(logLevel AVLogLevel) {
	C.av_log_set_level(C.int(logLevel))
	currentLoggingVerbosity = logLevel
}

type LoggingHandlerFunction func(messageLevel AVLogLevel, message string)

// SetLogging set AV logging handler
func SetLogging(handler LoggingHandlerFunction) {
	onceLogging.Do(func() {
		C.goavLogSetup()
	})
	if handler != nil {
		currentLoggingHandlerFunction = handler
	}
}

func noopLoggingHandler(_ AVLogLevel, _ string) {
}

func log(level AVLogLevel, message string) {
	if level <= currentLoggingVerbosity {
		currentLoggingHandlerFunction(level, message)
	}
}
