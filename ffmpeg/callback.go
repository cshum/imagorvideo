package ffmpeg

// #include "ffmpeg.h"
import "C"
import (
	"github.com/cshum/imagorvideo/ffmpeg/pointer"
	"io"
	"unsafe"
)

//export goPacketRead
func goPacketRead(opaque unsafe.Pointer, buf *C.uint8_t, bufSize C.int) C.int {
	ctx, ok := pointer.Restore(opaque).(*AVContext)
	if !ok || ctx.reader == nil {
		return C.int(ErrUnknown)
	}
	p := (*[1 << 30]byte)(unsafe.Pointer(buf))[:bufSize:bufSize]
	n, err := ctx.reader.Read(p)
	if err == io.EOF {
		return C.int(ErrEOF)
	} else if err != nil {
		return C.int(ErrUnknown)
	}
	return C.int(n)
}

//export goPacketSeek
func goPacketSeek(opaque unsafe.Pointer, offset C.int64_t, whence C.int) C.int64_t {
	ctx, ok := pointer.Restore(opaque).(*AVContext)
	if !ok || ctx.seeker == nil {
		return C.int64_t(ErrUnknown)
	}
	if whence == C.AVSEEK_SIZE {
		return C.int64_t(ctx.size)
	}
	n, err := ctx.seeker.Seek(int64(offset), int(whence))
	if err != nil {
		return C.int64_t(ErrUnknown)
	}
	return C.int64_t(n)
}

//export goInterrupt
func goInterrupt(opaque unsafe.Pointer) C.int {
	if ctx, ok := pointer.Restore(opaque).(*AVContext); ok {
		select {
		case <-ctx.context.Done():
			return 1
		default:
			return 0
		}
	}
	return 0
}

//export goAVLoggingHandler
func goAVLoggingHandler(level C.int, cstr *C.char) {
	log(AVLogLevel(level), C.GoString(cstr))
}
