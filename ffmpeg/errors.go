package ffmpeg

// #include "ffmpeg.h"
import "C"

type avError int

// AV Error enum
const (
	ErrNoMem           = avError(-C.ENOMEM)
	ErrEOF             = avError(C.AVERROR_EOF)
	ErrUnknown         = avError(C.AVERROR_UNKNOWN)
	ErrDecoderNotFound = avError(C.AVERROR_DECODER_NOT_FOUND)
	ErrInvalidData     = avError(C.AVERROR_INVALIDDATA)
	ErrTooBig          = avError(C.ERR_TOO_BIG)
)

func (e avError) errorString() string {
	switch e {
	case ErrNoMem:
		return "cannot allocate memory"
	case ErrTooBig:
		return "video or cover art size exceeds maximum allowed dimensions"
	case ErrEOF:
		return "end of file"
	case ErrDecoderNotFound:
		return "decoder not found"
	case ErrInvalidData:
		return "invalid data found when processing input"
	default:
		return "unknown error occurred"
	}
}

// Error implements error interface
func (e avError) Error() string {
	return "ffmpeg: " + e.errorString()
}
