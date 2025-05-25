package ffmpeg

import (
	"encoding/json"
	"fmt"
	"github.com/cshum/vipsgen/pointer"
	"github.com/cshum/vipsgen/vips"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
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
	"with_cover.mp3",
}

var noVideo = []string{
	"no_cover.mp3",
}

var baseDir = "../testdata/"

func TestAVContext(t *testing.T) {
	vips.Startup(nil)
	SetFFmpegLogLevel(AVLogDebug)
	logger := zap.NewExample()
	SetLogging(nil)
	log(AVLogDebug, "nop logging")
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
	require.NoError(t, os.MkdirAll(baseDir+"golden/export", 0755))
	t.Parallel()
	for _, filename := range files {
		for _, n := range []int{-1, 1, 5, 10, 9999, 99999} {
			name := filename
			if n > -1 {
				name = fmt.Sprintf("%s-%d", filename, n)
			}
			t.Run(name, func(t *testing.T) {
				path := baseDir + filename
				reader, err := os.Open(path)
				require.NoError(t, err)
				stats, err := os.Stat(path)
				require.NoError(t, err)
				av, err := LoadAVContext(reader, stats.Size())
				meta := av.Metadata()
				metaBuf, err := json.Marshal(meta)
				require.NoError(t, err)
				goldenFile := baseDir + "golden/meta/" + name + ".meta.json"
				if curr, err := os.ReadFile(goldenFile); err == nil {
					assert.Equal(t, string(curr), string(metaBuf))
				} else {
					require.NoError(t, os.WriteFile(goldenFile, metaBuf, 0666))
				}
				require.NoError(t, err)
				defer av.Close()
				if n == 10 {
					require.NoError(t, av.ProcessFrames(n))
				} else if n == 99999 {
					require.NoError(t, av.SelectDuration(time.Second))
				} else if n == 9999 {
					require.NoError(t, av.SelectPosition(0.7))
				} else if n == 1 {
					require.NoError(t, av.SelectDuration(0))
				} else if n == 5 {
					require.NoError(t, av.SelectFrame(n))
				} else {
					require.NoError(t, av.SeekPosition(0.7))
				}
				bands := 4
				if n == 99999 {
					bands = 999
				}
				buf, err := av.Export(bands)
				require.NoError(t, err)
				if bands > 4 {
					bands = 4
				}
				img, err := vips.NewImageFromMemory(buf, meta.Width, meta.Height, bands)
				require.NoError(t, err)
				buf, err = img.JpegsaveBuffer(nil)
				require.NoError(t, err)
				goldenFile = baseDir + "golden/export/" + name + ".jpg"
				if curr, err := os.ReadFile(goldenFile); err == nil {
					assert.True(t, reflect.DeepEqual(curr, buf))
				} else {
					require.NoError(t, os.WriteFile(goldenFile, buf, 0666))
				}
			})
		}
	}
}

func TestNoVideo(t *testing.T) {
	require.NoError(t, os.MkdirAll(baseDir+"golden/meta", 0755))
	require.NoError(t, os.MkdirAll(baseDir+"golden/export", 0755))
	for _, filename := range noVideo {
		for i := 0; i < 2; i++ {
			t.Run(fmt.Sprintf("%s-%d", filename, i), func(t *testing.T) {
				path := baseDir + filename
				reader, err := os.Open(path)
				require.NoError(t, err)
				stats, err := os.Stat(path)
				require.NoError(t, err)
				av, err := LoadAVContext(reader, stats.Size())
				require.NoError(t, err)
				defer av.Close()
				require.Equal(t, ErrDecoderNotFound, av.ProcessFrames(-1))
				meta := av.Metadata()
				metaBuf, err := json.Marshal(meta)
				require.NoError(t, err)
				goldenFile := baseDir + "golden/meta/" + filename + ".meta.json"
				if curr, err := os.ReadFile(goldenFile); err == nil {
					assert.Equal(t, string(curr), string(metaBuf))
				} else {
					require.NoError(t, os.WriteFile(goldenFile, metaBuf, 0666))
				}
				if i == 0 {
					buf, err := av.Export(3)
					require.Empty(t, buf)
					assert.Equal(t, ErrDecoderNotFound, err)
				} else {
					assert.Equal(t, ErrDecoderNotFound, av.SelectFrame(1))
				}
			})
		}
	}
}

func TestCorrupted(t *testing.T) {
	filename := "macabre.mp4"
	path := baseDir + filename
	file, err := os.Open(path)
	require.NoError(t, err)
	reader := &readCloser{
		Reader: io.LimitReader(file, 1024),
		Closer: file,
	}
	stats, err := os.Stat(path)
	require.NoError(t, err)
	av, err := LoadAVContext(reader, stats.Size())
	require.Equal(t, ErrInvalidData, err)
	require.Empty(t, av)
}

func TestCorruptedOpaque(t *testing.T) {
	filename := "macabre.mp4"
	path := baseDir + filename
	reader, err := os.Open(path)
	require.NoError(t, err)
	stats, err := os.Stat(path)
	require.NoError(t, err)
	av, err := LoadAVContext(reader, stats.Size())
	require.NoError(t, err)
	defer av.Close()
	pointer.Unref(av.opaque)
	err = av.ProcessFrames(-1)
	assert.Equal(t, ErrUnknown, err)
}

type readCloser struct {
	io.Reader
	io.Closer
}
