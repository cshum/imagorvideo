package ffmpeg

// #include "ffmpeg.h"
import "C"
import (
	"github.com/cshum/vipsgen/pointer"
	"io"
	"reflect"
	"unsafe"
)

//export goPacketRead
func goPacketRead(opaque unsafe.Pointer, buffer *C.uint8_t, bufSize C.int) C.int {
	ctx, ok := pointer.Restore(opaque).(*AVContext)
	if !ok || ctx.reader == nil {
		return C.int(ErrUnknown)
	}
	size := int(bufSize)
	sh := &reflect.SliceHeader{
		Data: uintptr(unsafe.Pointer(buffer)),
		Len:  size,
		Cap:  size,
	}
	buf := *(*[]byte)(unsafe.Pointer(sh))
	n, err := ctx.reader.Read(buf)
	if err == io.EOF {
		if n == 0 {
			return C.int(ErrEOF)
		}
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

//export goAVLoggingHandler
func goAVLoggingHandler(level C.int, cstr *C.char) {
	log(AVLogLevel(level), C.GoString(cstr))
}
