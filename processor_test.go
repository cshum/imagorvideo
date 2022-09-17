package imagorvideo

import (
	"context"
	"fmt"
	"github.com/cshum/imagor"
	"github.com/cshum/imagor/imagorpath"
	"github.com/cshum/imagor/storage/filestorage"
	"github.com/cshum/imagor/vips"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
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
	v := vips.NewProcessor(vips.WithDebug(true))
	require.NoError(t, v.Startup(context.Background()))
	t.Cleanup(func() {
		require.NoError(t, v.Shutdown(context.Background()))
	})
	doGoldenTests(t, filepath.Join(testDataDir, "golden/result"), []test{
		{name: "mkv", path: "fit-in/100x100/everybody-betray-me.mkv"},
		{name: "mkv meta", path: "meta/everybody-betray-me.mkv"},
		{name: "mp4", path: "fit-in/100x100/schizo_0.mp4"},
		{name: "mp4 90", path: "fit-in/100x100/schizo_90.mp4"},
		{name: "mp4 180", path: "fit-in/100x100/schizo_180.mp4"},
		{name: "mp4 270", path: "fit-in/100x100/schizo_270.mp4"},
		{name: "image", path: "fit-in/100x100/demo.png"},
		{name: "corrupted", path: "fit-in/100x100/corrupt/everybody-betray-me.mkv", expectCode: 406},
	}, WithDebug(true), WithLogger(zap.NewExample()))
	doGoldenTests(t, filepath.Join(testDataDir, "golden/result-fallback-image"), []test{
		{name: "corrupted with fallback image", path: "fit-in/100x100/corrupt/everybody-betray-me.mkv", expectCode: 406},
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
			imagor.WithProcessors(NewProcessor(opts...), vips.NewProcessor()),
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
				_ = resStorage.Put(context.Background(), tt.path, b)
				path := filepath.Join(resultDir, imagorpath.Normalize(tt.path, nil))

				bc := imagor.NewBlobFromFile(path)
				buf, err := bc.ReadAll()
				require.NoError(t, err)
				assert.True(t, reflect.DeepEqual(buf, w.Body.Bytes()))
			})
		}

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
