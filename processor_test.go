package imagorvideo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/cshum/imagor"
	"github.com/cshum/imagor/imagorpath"
	"github.com/cshum/imagor/processor/vipsprocessor"
	"github.com/cshum/imagor/storage/filestorage"
	"github.com/cshum/vipsgen/vips"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var testDataDir string

func init() {
	_, b, _, _ := runtime.Caller(0)
	testDataDir = filepath.Join(filepath.Dir(b), "./testdata")
}

type test struct {
	name       string
	path       string
	expectCode int
}

func TestProcessor(t *testing.T) {
	v := vipsprocessor.NewProcessor(vipsprocessor.WithDebug(true))
	require.NoError(t, v.Startup(context.Background()))
	t.Cleanup(func() {
		require.NoError(t, v.Shutdown(context.Background()))
	})
	doGoldenTests(t, filepath.Join(testDataDir, "golden/result"), []test{
		{name: "mkv", path: "fit-in/100x100/everybody-betray-me.mkv"},
		{name: "mkv specific frame", path: "fit-in/100x100/filters:frame(3)/everybody-betray-me.mkv"},
		{name: "mkv specific max_frames", path: "fit-in/100x100/filters:max_frames(6)/everybody-betray-me.mkv"},
		{name: "mkv specific frame exceeded", path: "fit-in/100x100/filters:frame(99999)/everybody-betray-me.mkv"},
		{name: "mkv meta max_frames", path: "meta/filters:max_frames()/everybody-betray-me.mkv"},
		{name: "mkv meta max_frames 6", path: "meta/filters:max_frames(6)/everybody-betray-me.mkv"},
		{name: "mkv meta", path: "meta/everybody-betray-me.mkv"},
		{name: "mp4", path: "200x100/schizo_0.mp4"},
		{name: "mp4 orient 90", path: "220x100/schizo_90.mp4"},
		{name: "mp4 orient 180", path: "200x100/schizo_180.mp4"},
		{name: "mp4 orient 270", path: "200x100/schizo_270.mp4"},
		{name: "image", path: "fit-in/100x100/demo.png"},
		{name: "alpha", path: "fit-in/filters:format(png)/alpha-webm.webm"},
		{name: "alpha frame duration", path: "500x/filters:frame(5s):format(png)/alpha-webm.webm"},
		{name: "alpha frame position", path: "500x/filters:frame(0.5):format(png)/alpha-webm.webm"},
		{name: "alpha seek duration", path: "500x/filters:seek(5s):format(png)/alpha-webm.webm"},
		{name: "alpha seek position", path: "500x/filters:seek(0.5):format(png)/alpha-webm.webm"},
		{name: "corrupted", path: "fit-in/100x100/corrupt/everybody-betray-me.mkv", expectCode: 406},
		{name: "no cover meta", path: "meta/no_cover.mp3"},
		{name: "no cover 406", path: "fit-in/100x100/no_cover.mp3", expectCode: 406},
	}, WithDebug(true), WithLogger(zap.NewExample()))
	doGoldenTests(t, filepath.Join(testDataDir, "golden/result-fallback-image"), []test{
		{name: "corrupted with fallback image", path: "fit-in/100x100/corrupt/everybody-betray-me.mkv", expectCode: 406},
		{name: "corrupted with fallback image", path: "filters:seek(0.1)/no_cover.mp3", expectCode: 406},
	}, WithDebug(false), WithLogger(zap.NewExample()), WithFallbackImage("demo.png"))
}

func doGoldenTests(t *testing.T, resultDir string, tests []test, opts ...Option) {
	resStorage := filestorage.New(resultDir,
		filestorage.WithSaveErrIfExists(true))
	fileLoader := filestorage.New(testDataDir)
	loaders := []imagor.Loader{
		fileLoader,
		loaderFunc(func(r *http.Request, image string) (blob *imagor.Blob, err error) {
			image, _ = fileLoader.Path(image)
			return imagor.NewBlob(func() (reader io.ReadCloser, size int64, err error) {
				// force read full file by 0 size
				reader, err = os.Open(image)
				return
			}), nil
		}),
	}
	for i, loader := range loaders {
		app := imagor.New(
			imagor.WithLoaders(loaderFunc(func(r *http.Request, image string) (blob *imagor.Blob, err error) {
				if strings.HasPrefix(image, "corrupt/") {
					image, _ = fileLoader.Path(strings.TrimPrefix(image, "corrupt/"))
					return imagor.NewBlob(func() (reader io.ReadCloser, size int64, err error) {
						file, err := os.Open(image)
						// truncate so it corrupt
						reader = &readCloser{
							Reader: io.LimitReader(file, 1024),
							Closer: file,
						}
						return
					}), nil
				}
				return nil, imagor.ErrNotFound
			}), loader),
			imagor.WithUnsafe(true),
			imagor.WithDebug(true),
			imagor.WithLogger(zap.NewExample()),
			imagor.WithProcessors(NewProcessor(opts...), vipsprocessor.NewProcessor()),
		)
		require.NoError(t, app.Startup(context.Background()))
		t.Cleanup(func() {
			assert.NoError(t, app.Shutdown(context.Background()))
		})
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%s-%d", tt.name, i+1), func(t *testing.T) {
				w := httptest.NewRecorder()
				ctx, cancel := context.WithCancel(context.Background())
				req := httptest.NewRequest(
					http.MethodGet, fmt.Sprintf("/unsafe/%s", tt.path), nil).WithContext(ctx)
				app.ServeHTTP(w, req)
				cancel()
				if tt.expectCode == 0 {
					assert.Equal(t, 200, w.Code)
				} else {
					assert.Equal(t, tt.expectCode, w.Code)
				}
				b := imagor.NewBlobFromBytes(w.Body.Bytes())
				path := tt.path
				if strings.HasPrefix(path, "meta/") {
					path += ".json"
				}
				_ = resStorage.Put(context.Background(), path, b)
				path = filepath.Join(resultDir, imagorpath.Normalize(path, nil))
				bc := imagor.NewBlobFromFile(path)
				buf, err := bc.ReadAll()
				require.NoError(t, err)
				if reflect.DeepEqual(buf, w.Body.Bytes()) {
					return
				}
				img1, err := vips.NewImageFromBuffer(buf, nil)
				require.NoError(t, err)
				img2, err := vips.NewImageFromBuffer(w.Body.Bytes(), nil)
				require.NoError(t, err)
				require.Equal(t, img1.Width(), img2.Width(), "width mismatch")
				require.Equal(t, img1.Height(), img2.Height(), "height mismatch")
				buf1, err := img1.WebpsaveBuffer(nil)
				require.NoError(t, err)
				buf2, err := img2.WebpsaveBuffer(nil)
				require.NoError(t, err)
				require.True(t, reflect.DeepEqual(buf1, buf2), "image mismatch")
			})
		}

	}

}

// TestProcessorRawBypass verifies that camera RAW formats are forwarded to the next
// processor (vipsprocessor) rather than being decoded by ffmpeg.
// CR3 (Canon RAW 3) is an ISO BMFF container that mimetype may misdetect as video.
func TestProcessorRawBypass(t *testing.T) {
	p := NewProcessor(WithLogger(zap.NewExample()))
	require.NoError(t, p.Startup(context.Background()))
	defer func() {
		require.NoError(t, p.Shutdown(context.Background()))
	}()

	// Construct minimal synthetic RAW blobs from magic bytes — no testdata files needed.
	// CR3: ISO BMFF container with "ftyp" at offset 4 and "crx " at offset 8.
	// This is the format that mimetype misdetects as video/mp4.
	makeCR3 := func() []byte {
		buf := make([]byte, 512)
		copy(buf[4:], []byte("ftyp"))
		copy(buf[8:], []byte("crx "))
		return buf
	}
	// CR2: TIFF-based with "CR" magic at offset 8.
	makeCR2 := func() []byte {
		buf := make([]byte, 512)
		copy(buf[0:], []byte("\x49\x49\x2A\x00")) // TIFF little-endian
		copy(buf[8:], []byte("CR"))
		return buf
	}
	// RAF: Fuji RAF with "FUJIFILMCCD-RAW" header.
	makeRAF := func() []byte {
		buf := make([]byte, 512)
		copy(buf[0:], []byte("FUJIFILMCCD-RAW"))
		return buf
	}

	tests := []struct {
		name string
		buf  []byte
	}{
		{name: "CR3 forwarded (ISO BMFF misdetected as video)", buf: makeCR3()},
		{name: "CR2 forwarded (TIFF-based RAW)", buf: makeCR2()},
		{name: "RAF forwarded (Fuji RAW)", buf: makeRAF()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blob := imagor.NewBlobFromBytes(tt.buf)
			assert.True(t, blob.IsRaw(), "blob should be detected as RAW")

			out, err := p.Process(context.Background(), blob, imagorpath.Params{}, nil)

			// Must return ErrForward — not a 406 error
			var fwd imagor.ErrForward
			require.ErrorAs(t, err, &fwd, "RAW file must be forwarded, not processed by ffmpeg")
			assert.Equal(t, blob, out, "forwarded blob should be the original blob unchanged")
		})
	}
}

type loaderFunc func(r *http.Request, image string) (blob *imagor.Blob, err error)

func (f loaderFunc) Get(r *http.Request, image string) (*imagor.Blob, error) {
	return f(r, image)
}

type readCloser struct {
	io.Reader
	io.Closer
}
