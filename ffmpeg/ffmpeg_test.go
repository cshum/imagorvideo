package ffmpeg

import (
	"context"
	"encoding/json"
	"github.com/cshum/imagor/vips"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"reflect"
	"testing"
)

var files = []struct {
	file string
}{
	{file: "everybody-betray-me.mkv"},
	{file: "alpha-webm.webm"},
	{file: "schizo.flv"},
	{file: "macabre.mp4"},
	{file: "schizo_0.mp4"},
	{file: "schizo_90.mp4"},
	{file: "schizo_180.mp4"},
	{file: "schizo_270.mp4"},
}

var baseDir = "../testdata/"

func TestAVContextMeta(t *testing.T) {
	vips.Startup(nil)
	require.NoError(t, os.MkdirAll(baseDir+"golden/meta", 0755))
	require.NoError(t, os.MkdirAll(baseDir+"golden/result", 0755))
	t.Parallel()
	for _, tt := range files {
		t.Run(tt.file, func(t *testing.T) {
			ctx := context.Background()
			path := baseDir + tt.file
			reader, err := os.Open(path)
			require.NoError(t, err)
			stats, err := os.Stat(path)
			require.NoError(t, err)
			av, err := LoadAVContext(ctx, reader, stats.Size())
			require.NoError(t, err)

			meta := av.Metadata()
			metaBuf, err := json.Marshal(meta)
			require.NoError(t, err)
			goldenFile := baseDir + "golden/meta/" + tt.file + ".meta.json"
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
			goldenFile = baseDir + "golden/result/" + tt.file + ".jpg"
			if curr, err := os.ReadFile(goldenFile); err == nil {
				assert.True(t, reflect.DeepEqual(curr, buf))
			} else {
				require.NoError(t, os.WriteFile(goldenFile, buf, 0666))
			}

		})
	}
}
