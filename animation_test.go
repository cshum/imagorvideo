package imagorvideo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAnimation(t *testing.T) {
	tests := []struct {
		args   string
		ok     bool
		frames int
		clip   time.Duration
		fps    float64
	}{
		{args: "10", ok: true, frames: 10, fps: defaultAnimationFPS},
		{args: "10,2", ok: true, frames: 10, fps: 2},
		{args: "10, 2.5", ok: true, frames: 10, fps: 2.5},
		{args: "3s", ok: true, clip: 3 * time.Second, fps: defaultAnimationFPS},
		{args: "500ms,12", ok: true, clip: 500 * time.Millisecond, fps: 12},
		{args: ""},
		{args: "0"},
		{args: "-3"},
		{args: "abc"},
		{args: "10,0"},
		{args: "10,x"},
		{args: "10,2,3"},
		{args: "-2s"},
	}
	for _, tt := range tests {
		t.Run(tt.args, func(t *testing.T) {
			a, ok := parseAnimation(tt.args)
			assert.Equal(t, tt.ok, ok)
			if !ok {
				return
			}
			assert.Equal(t, tt.frames, a.frames)
			assert.Equal(t, tt.clip, a.clip)
			assert.Equal(t, tt.fps, a.fps)
		})
	}
}

func TestAnimationTimestamps(t *testing.T) {
	t.Run("frames spread over video", func(t *testing.T) {
		a := animation{frames: 4, fps: 5}
		ts, err := a.timestamps(0, 8*time.Second, 100)
		require.NoError(t, err)
		assert.Equal(t, []time.Duration{
			1 * time.Second, 3 * time.Second, 5 * time.Second, 7 * time.Second,
		}, ts)
	})
	t.Run("frames from seek position", func(t *testing.T) {
		a := animation{frames: 2, fps: 5}
		ts, err := a.timestamps(4*time.Second, 8*time.Second, 100)
		require.NoError(t, err)
		assert.Equal(t, []time.Duration{5 * time.Second, 7 * time.Second}, ts)
	})
	t.Run("frames seek beyond duration restarts", func(t *testing.T) {
		a := animation{frames: 2, fps: 5}
		ts, err := a.timestamps(9*time.Second, 8*time.Second, 100)
		require.NoError(t, err)
		assert.Equal(t, []time.Duration{2 * time.Second, 6 * time.Second}, ts)
	})
	t.Run("frames capped", func(t *testing.T) {
		a := animation{frames: 50, fps: 5}
		ts, err := a.timestamps(0, 8*time.Second, 3)
		require.NoError(t, err)
		assert.Len(t, ts, 3)
	})
	t.Run("frames need duration", func(t *testing.T) {
		a := animation{frames: 4, fps: 5}
		_, err := a.timestamps(0, 0, 100)
		assert.ErrorIs(t, err, errAnimationDuration)
	})
	t.Run("clip at fps", func(t *testing.T) {
		a := animation{clip: time.Second, fps: 4}
		ts, err := a.timestamps(2*time.Second, 0, 100)
		require.NoError(t, err)
		assert.Equal(t, []time.Duration{
			2000 * time.Millisecond, 2250 * time.Millisecond, 2500 * time.Millisecond, 2750 * time.Millisecond,
		}, ts)
	})
	t.Run("clip shorter than one frame", func(t *testing.T) {
		a := animation{clip: 10 * time.Millisecond, fps: 4}
		ts, err := a.timestamps(0, 0, 100)
		require.NoError(t, err)
		assert.Equal(t, []time.Duration{0}, ts)
	})
	t.Run("clip capped", func(t *testing.T) {
		a := animation{clip: time.Minute, fps: 1000}
		ts, err := a.timestamps(0, 0, 7)
		require.NoError(t, err)
		assert.Len(t, ts, 7)
		assert.Equal(t, minAnimationFrameDelay, ts[1]-ts[0])
	})
}
