package imagorvideo

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/cshum/imagor"
	"github.com/cshum/imagorvideo/ffmpeg"
	"github.com/cshum/vipsgen/vips"
)

const (
	defaultAnimationFPS       = 5.0
	defaultMaxAnimationFrames = 100
	minAnimationFrameDelay    = 20 * time.Millisecond
)

var errAnimationDuration = errors.New("imagorvideo: video duration unknown, use a clip duration instead of frame count")

// animation holds the parsed arguments of the gif filter
type animation struct {
	// frames evenly spread over the video, exclusive with clip
	frames int
	// clip of consecutive playback starting at the seek position
	clip time.Duration
	fps  float64
}

// parseAnimation parses gif filter arguments in the forms
// n, n,fps, duration and duration,fps
func parseAnimation(args string) (a animation, ok bool) {
	a.fps = defaultAnimationFPS
	parts := strings.Split(args, ",")
	if len(parts) > 2 {
		return
	}
	first := strings.TrimSpace(parts[0])
	if d, err := time.ParseDuration(first); err == nil && d > 0 {
		a.clip = d
	} else if n, err := strconv.Atoi(first); err == nil && n > 0 {
		a.frames = n
	} else {
		return
	}
	if len(parts) == 2 {
		fps, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil || fps <= 0 || math.IsInf(fps, 0) {
			return
		}
		a.fps = fps
	}
	return a, true
}

// timestamps returns the frame presentation times to export, starting at
// start and bounded by duration when known
func (a animation) timestamps(start, duration time.Duration, maxFrames int) ([]time.Duration, error) {
	if a.frames > 0 {
		if duration <= 0 {
			return nil, errAnimationDuration
		}
		if start >= duration {
			start = 0
		}
		n := a.frames
		if n > maxFrames {
			n = maxFrames
		}
		span := duration - start
		ts := make([]time.Duration, n)
		for i := range ts {
			// center each sample in its slot to avoid the black frames at the start
			ts[i] = start + time.Duration(float64(span)*(float64(i)+0.5)/float64(n))
		}
		return ts, nil
	}
	step := a.frameDelay()
	n := int(a.clip / step)
	if n < 1 {
		n = 1
	}
	if n > maxFrames {
		n = maxFrames
	}
	ts := make([]time.Duration, n)
	for i := range ts {
		ts[i] = start + time.Duration(i)*step
	}
	return ts, nil
}

func (a animation) frameDelay() time.Duration {
	delay := time.Duration(float64(time.Second) / a.fps)
	if delay < minAnimationFrameDelay {
		delay = minAnimationFrameDelay
	}
	return delay
}

// exportAnimation decodes the frames and joins them into an animated WebP
// blob, which the vips processor understands as a multi page image
func exportAnimation(av *ffmpeg.AVContext, meta *ffmpeg.Metadata, start time.Duration, a animation, bands, maxFrames int) (*imagor.Blob, error) {
	timestamps, err := a.timestamps(start, time.Duration(meta.Duration)*time.Millisecond, maxFrames)
	if err != nil {
		return nil, err
	}
	frames, err := av.ExportFrames(timestamps, bands)
	if err != nil {
		return nil, err
	}
	if len(frames) == 0 {
		return nil, imagor.ErrUnsupportedFormat
	}
	frameSize := meta.Width * meta.Height * bands
	images := make([]*vips.Image, 0, len(frames))
	defer func() {
		for _, img := range images {
			img.Close()
		}
	}()
	for _, buf := range frames {
		if len(buf) != frameSize {
			return nil, ffmpeg.ErrInvalidData
		}
		img, err := vips.NewImageFromMemory(buf, meta.Width, meta.Height, bands)
		if err != nil {
			return nil, err
		}
		images = append(images, img)
	}
	joined, err := vips.NewArrayjoin(images, &vips.ArrayjoinOptions{Across: 1})
	if err != nil {
		return nil, err
	}
	defer joined.Close()
	// NewImageFromMemory assigns the multiband interpretation, which webpsave rejects
	page, err := joined.Copy(&vips.CopyOptions{Interpretation: vips.InterpretationSrgb})
	if err != nil {
		return nil, err
	}
	defer page.Close()
	if err = page.SetPageHeight(meta.Height); err != nil {
		return nil, err
	}
	delays := make([]int, len(frames))
	for i := range delays {
		delays[i] = int(a.frameDelay() / time.Millisecond)
	}
	if err = page.SetArrayInt("delay", delays); err != nil {
		return nil, err
	}
	page.SetInt("loop", 0)
	options := vips.DefaultWebpsaveBufferOptions()
	options.Lossless = true
	options.Effort = 0
	buf, err := page.WebpsaveBuffer(options)
	if err != nil {
		return nil, err
	}
	return imagor.NewBlobFromBytes(buf), nil
}
