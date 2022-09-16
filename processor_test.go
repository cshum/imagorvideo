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
	"testing"
)

var testDataDir string

func init() {
	_, b, _, _ := runtime.Caller(0)
	testDataDir = filepath.Join(filepath.Dir(b), "./testdata")
}

type test struct {
	name string
	path string
}

func TestProcessor(t *testing.T) {
	v := vips.NewProcessor(vips.WithDebug(true))
	require.NoError(t, v.Startup(context.Background()))
	t.Cleanup(func() {
		require.NoError(t, v.Shutdown(context.Background()))
	})
	var resultDir = filepath.Join(testDataDir, "golden/result")
	doGoldenTests(t, resultDir, []test{
		{name: "mkv", path: "fit-in/100x100/everybody-betray-me.mkv"},
		{name: "mkv meta", path: "meta/everybody-betray-me.mkv"},
		{name: "mp4", path: "fit-in/100x100/schizo_0.mp4"},
		{name: "mp4 90", path: "fit-in/100x100/schizo_90.mp4"},
		{name: "mp4 180", path: "fit-in/100x100/schizo_180.mp4"},
		{name: "mp4 270", path: "fit-in/100x100/schizo_270.mp4"},
		{name: "image", path: "fit-in/100x100/demo.png"},
	}, WithDebug(true))
}

func doGoldenTests(t *testing.T, resultDir string, tests []test, opts ...Option) {
	resStorage := filestorage.New(resultDir,
		filestorage.WithSaveErrIfExists(true))
	loader := filestorage.New(testDataDir)
	loaders := []imagor.Loader{
		loader,
		loaderFunc(func(r *http.Request, image string) (blob *imagor.Blob, err error) {
			image, _ = loader.Path(image)
			return imagor.NewBlob(func() (reader io.ReadCloser, size int64, err error) {
				// force read full file by 0 size
				reader, err = os.Open(image)
				return
			}), nil
		}),
	}
	for i, loader := range loaders {
		app := imagor.New(
			imagor.WithLoaders(loader),
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
				assert.Equal(t, 200, w.Code)
				b := imagor.NewBlobFromBytes(w.Body.Bytes())
				_ = resStorage.Put(context.Background(), tt.path, b)
				path := filepath.Join(resultDir, imagorpath.Normalize(tt.path, nil))

				bc := imagor.NewBlobFromFile(path)
				buf, err := bc.ReadAll()
				require.NoError(t, err)
				if reflect.DeepEqual(buf, w.Body.Bytes()) {
					return
				}
				img1, err := vips.LoadImageFromFile(path, nil)
				require.NoError(t, err)
				img2, err := vips.LoadImageFromBuffer(w.Body.Bytes(), nil)
				require.NoError(t, err)
				require.Equal(t, img1.Width(), img2.Width(), "width mismatch")
				require.Equal(t, img1.Height(), img2.Height(), "height mismatch")
				buf1, err := img1.ExportWebp(nil)
				require.NoError(t, err)
				buf2, err := img2.ExportWebp(nil)
				require.NoError(t, err)
				require.True(t, reflect.DeepEqual(buf1, buf2), "image mismatch")
			})
		}

	}

}

type loaderFunc func(r *http.Request, image string) (blob *imagor.Blob, err error)

func (f loaderFunc) Get(r *http.Request, image string) (*imagor.Blob, error) {
	return f(r, image)
}
