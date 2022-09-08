package ffmpeg

// #include "ffmpeg.h"
import "C"
import (
	"strconv"
	"unsafe"
)

type avError int

const (
	ErrNoMem           = avError(-C.ENOMEM)
	ErrEOF             = avError(C.AVERROR_EOF)
	ErrUnknown         = avError(C.AVERROR_UNKNOWN)
	ErrDecoderNotFound = avError(C.AVERROR_DECODER_NOT_FOUND)
	ErrInvalidData     = avError(C.AVERROR_INVALIDDATA)
	ErrTooBig          = avError(C.ERR_TOO_BIG)
)

func (e avError) errorString() string {
	if e == ErrNoMem {
		return "cannot allocate memory"
	}
	if e == ErrTooBig {
		return "video or cover art size exceeds maximum allowed dimensions"
	}
	errString := (*C.char)(C.av_malloc(C.AV_ERROR_MAX_STRING_SIZE))
	if errString == nil {
		return "cannot allocate memory for error string, error code: " + strconv.Itoa(int(e))
	}
	defer C.av_free(unsafe.Pointer(errString))
	C.av_make_error_string(errString, C.AV_ERROR_MAX_STRING_SIZE, C.int(e))
	return C.GoString(errString)
}

func (e avError) Error() string {
	return "ffmpeg: " + e.errorString()
}
