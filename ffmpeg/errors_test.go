package ffmpeg

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestErrors(t *testing.T) {
	assert.Equal(t, "ffmpeg: cannot allocate memory", ErrNoMem.Error())
	assert.Equal(t, "ffmpeg: end of file", ErrEOF.Error())
	assert.Equal(t, "ffmpeg: unknown error occurred", ErrUnknown.Error())
	assert.Equal(t, "ffmpeg: decoder not found", ErrDecoderNotFound.Error())
	assert.Equal(t, "ffmpeg: invalid data found when processing input", ErrInvalidData.Error())
	assert.Equal(t, "ffmpeg: video or cover art size exceeds maximum allowed dimensions", ErrTooBig.Error())
}
