package imagorvideo

import (
	"github.com/cshum/imagor"
	"github.com/cshum/imagor/config"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestConfig(t *testing.T) {
	srv := config.CreateServer([]string{
		"-ffmpeg-fallback-image", "https://foo.com/bar.jpg",
	}, Config)
	app := srv.App.(*imagor.Imagor)
	processor := app.Processors[0].(*Processor)
	assert.Equal(t, "https://foo.com/bar.jpg", processor.FallbackImage)
}
