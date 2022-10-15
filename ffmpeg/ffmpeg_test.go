package ffmpeg

import (
	"encoding/json"
	"fmt"
	"github.com/cshum/imagor/vips"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"io"
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
		for _, frame := range []int{-1, 5, 10, 9999, 99999} {
			name := filename
			if frame > -1 {
				name = fmt.Sprintf("%s-%d", filename, frame)
			}
			t.Run(name, func(t *testing.T) {
				path := baseDir + filename
				reader, err := os.Open(path)
				require.NoError(t, err)
				stats, err := os.Stat(path)
				require.NoError(t, err)
				av, err := LoadAVContext(reader, stats.Size())
				require.NoError(t, err)
				defer av.Close()
				if frame == 10 {
					require.NoError(t, av.ProcessFrames(frame))
				} else {
					if frame == 9999 {
						require.NoError(t, av.ProcessFrames(-1))
					}
					if frame > -1 {
						require.NoError(t, av.SelectFrame(frame))
					}
				}
				meta := av.Metadata()
				metaBuf, err := json.Marshal(meta)
				require.NoError(t, err)
				goldenFile := baseDir + "golden/meta/" + name + ".meta.json"
				if curr, err := os.ReadFile(goldenFile); err == nil {
					assert.Equal(t, string(curr), string(metaBuf))
				} else {
					require.NoError(t, os.WriteFile(goldenFile, metaBuf, 0666))
				}
				bands := 4
				buf, err := av.Export(bands)
				require.NoError(t, err)
				img, err := vips.LoadImageFromMemory(buf, meta.Width, meta.Height, bands)
				require.NoError(t, err)
				buf, err = img.ExportJpeg(nil)
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
		t.Run(filename, func(t *testing.T) {
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
			buf, err := av.Export(3)
			require.Empty(t, buf)
			require.Equal(t, ErrDecoderNotFound, err)
		})
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

type readCloser struct {
	io.Reader
	io.Closer
}
