package ffmpeg

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cshum/vipsgen/vips"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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

func TestExportFrames(t *testing.T) {
	vips.Startup(nil)
	require.NoError(t, os.MkdirAll(baseDir+"golden/frames", 0755))
	for _, filename := range files {
		t.Run(filename, func(t *testing.T) {
			path := baseDir + filename
			reader, err := os.Open(path)
			require.NoError(t, err)
			stats, err := os.Stat(path)
			require.NoError(t, err)
			av, err := LoadAVContext(reader, stats.Size())
			require.NoError(t, err)
			defer av.Close()
			meta := av.Metadata()
			duration := time.Duration(meta.Duration) * time.Millisecond
			// mix of sequential decode within the window and a seek across it
			timestamps := []time.Duration{
				duration / 4, duration/4 + 200*time.Millisecond, duration * 3 / 4,
			}
			frames, err := av.ExportFrames(timestamps, 3)
			require.NoError(t, err)
			require.NotEmpty(t, frames)
			require.LessOrEqual(t, len(frames), len(timestamps))
			for i, buf := range frames {
				require.Len(t, buf, meta.Width*meta.Height*3)
				img, err := vips.NewImageFromMemory(buf, meta.Width, meta.Height, 3)
				require.NoError(t, err)
				jpg, err := img.JpegsaveBuffer(nil)
				require.NoError(t, err)
				goldenFile := fmt.Sprintf("%sgolden/frames/%s-%d.jpg", baseDir, filename, i)
				if curr, err := os.ReadFile(goldenFile); err == nil {
					assert.True(t, reflect.DeepEqual(curr, jpg), goldenFile)
				} else {
					require.NoError(t, os.WriteFile(goldenFile, jpg, 0666))
				}
			}
			// seeking backwards after a forward run must work too
			again, err := av.ExportFrames([]time.Duration{0}, 4)
			require.NoError(t, err)
			require.Len(t, again, 1)
			require.Len(t, again[0], meta.Width*meta.Height*4)
			// timestamps beyond the end return what could be decoded
			late, err := av.ExportFrames([]time.Duration{duration * 10}, 3)
			require.NoError(t, err)
			require.LessOrEqual(t, len(late), 1)
		})
	}
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
			frames, err := av.ExportFrames([]time.Duration{0}, 3)
			assert.Empty(t, frames)
			assert.Equal(t, ErrDecoderNotFound, err)
		})
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
	invalidateOpaqueHandle(av.opaque)
	err = av.ProcessFrames(-1)
	assert.Equal(t, ErrUnknown, err)
}

type readCloser struct {
	io.Reader
	io.Closer
}

func TestAudioWithCover(t *testing.T) {
	path := baseDir + "with_cover.mp3"
	reader, err := os.Open(path)
	require.NoError(t, err)
	stats, err := os.Stat(path)
	require.NoError(t, err)

	av, err := LoadAVContext(reader, stats.Size())
	require.NoError(t, err)
	defer av.Close()

	meta := av.Metadata()
	require.True(t, meta.HasAudio)
	require.True(t, meta.HasVideo)
	require.Greater(t, meta.Width, 0)
	require.Greater(t, meta.Height, 0)

	require.NoError(t, av.SelectFrame(1))
	buf, err := av.Export(4)
	require.NoError(t, err)
	require.NotEmpty(t, buf)

	img, err := vips.NewImageFromMemory(buf, meta.Width, meta.Height, 4)
	require.NoError(t, err)
	defer img.Close()
	_, err = img.JpegsaveBuffer(nil)
	require.NoError(t, err)
}

func TestAudioOnlyNoSeekEdgeCases(t *testing.T) {
	path := baseDir + "no_cover.mp3"
	file, err := os.Open(path)
	require.NoError(t, err)
	stats, err := os.Stat(path)
	require.NoError(t, err)

	reader := &readCloser{
		Reader: file,
		Closer: file,
	}

	av, err := LoadAVContext(reader, stats.Size())
	require.NoError(t, err)
	defer av.Close()

	require.Nil(t, av.seeker)

	meta := av.Metadata()
	require.True(t, meta.HasAudio)
	require.False(t, meta.HasVideo)
	require.Zero(t, meta.Width)
	require.Zero(t, meta.Height)

	assert.Equal(t, ErrDecoderNotFound, av.ProcessFrames(-1))
	assert.Equal(t, ErrDecoderNotFound, av.SelectFrame(1))
	assert.Equal(t, ErrDecoderNotFound, av.SelectDuration(time.Second))
	assert.Equal(t, ErrDecoderNotFound, av.SelectPosition(0.5))
	assert.Equal(t, ErrDecoderNotFound, av.SeekDuration(time.Second))
	assert.Equal(t, ErrDecoderNotFound, av.SeekPosition(0.5))

	buf, err := av.Export(4)
	require.Empty(t, buf)
	assert.Equal(t, ErrDecoderNotFound, err)

	av.Close()
}
