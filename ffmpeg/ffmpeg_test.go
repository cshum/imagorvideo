package ffmpeg

import (
	"context"
	"encoding/json"
	"github.com/cshum/imagor/vips"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"os"
	"reflect"
	"strings"
	"testing"
)

var files = []string{
	"everybody-betray-me.mkv",
	"alpha-webm.webm",
	"schizo.flv",
	"macabre.mp4",
	"schizo_0.mp4",
	"schizo_90.mp4",
	"schizo_180.mp4",
	"schizo_270.mp4",
}

var baseDir = "../testdata/"

func TestAVContextMeta(t *testing.T) {
	vips.Startup(nil)
	SetFFmpegLogLevel(AVLogDebug)
	logger := zap.NewExample()
	SetLogging(nil)
	SetLogging(func(level AVLogLevel, message string) {
		message = strings.TrimSuffix(message, "\n")
		switch level {
		case AVLogTrace, AVLogDebug, AVLogVerbose:
			logger.Debug("ffmpeg", zap.String("log", message))
		case AVLogInfo:
			logger.Info("ffmpeg", zap.String("log", message))
		case AVLogWarning, AVLogError, AVLogFatal, AVLogPanic:
			logger.Warn("ffmpeg", zap.String("log", message))
		}
	})
	require.NoError(t, os.MkdirAll(baseDir+"golden/meta", 0755))
	require.NoError(t, os.MkdirAll(baseDir+"golden/result", 0755))
	t.Parallel()
	for _, filename := range files {
		t.Run(filename, func(t *testing.T) {
			ctx := context.Background()
			path := baseDir + filename
			reader, err := os.Open(path)
			require.NoError(t, err)
			stats, err := os.Stat(path)
			require.NoError(t, err)
			av, err := LoadAVContext(ctx, reader, stats.Size())
			require.NoError(t, err)
			defer av.Close()

			meta := av.Metadata()
			metaBuf, err := json.Marshal(meta)
			require.NoError(t, err)
			goldenFile := baseDir + "golden/meta/" + filename + ".meta.json"
			if curr, err := os.ReadFile(goldenFile); err == nil {
				assert.Equal(t, string(curr), string(metaBuf))
			} else {
				require.NoError(t, os.WriteFile(goldenFile, metaBuf, 0666))
			}

			buf, err := av.Export()
			require.NoError(t, err)
			bands := 3
			if meta.HasAlpha {
				bands = 4
			}
			img, err := vips.LoadImageFromMemory(buf, meta.Width, meta.Height, bands)
			require.NoError(t, err)
			buf, err = img.ExportJpeg(nil)
			require.NoError(t, err)
			goldenFile = baseDir + "golden/export/" + filename + ".jpg"
			if curr, err := os.ReadFile(goldenFile); err == nil {
				assert.True(t, reflect.DeepEqual(curr, buf))
			} else {
				require.NoError(t, os.WriteFile(goldenFile, buf, 0666))
			}

		})
	}
}

func TestErrors(t *testing.T) {
	assert.Equal(t, "ffmpeg: cannot allocate memory", ErrNoMem.Error())
	assert.Equal(t, "ffmpeg: end of file", ErrEOF.Error())
	assert.Equal(t, "ffmpeg: unknown error occurred", ErrUnknown.Error())
	assert.Equal(t, "ffmpeg: decoder not found", ErrDecoderNotFound.Error())
	assert.Equal(t, "ffmpeg: invalid data found when processing input", ErrInvalidData.Error())
	assert.Equal(t, "ffmpeg: video or cover art size exceeds maximum allowed dimensions", ErrTooBig.Error())
}
