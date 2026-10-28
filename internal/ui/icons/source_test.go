package icons

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	qt "github.com/mappu/miqt/qt6"
	"github.com/stretchr/testify/suite"
)

type SourceSuite struct{ suite.Suite }

func TestSourceSuite(t *testing.T) { suite.Run(t, new(SourceSuite)) }

func (s *SourceSuite) TestCacheEvictionNegativeEntriesAndExplicitRefresh() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app := qt.NewQApplication([]string{"icon-test", "-platform", "offscreen"})
	defer app.Delete()
	source := New(nil, nil)
	directory := s.T().TempDir()
	path := filepath.Join(directory, "icon.png")
	s.Require().NoError(os.WriteFile(path, []byte("corrupt"), 0o600))
	fallback := source.URL("", path)
	s.Equal(source.URL("", ""), fallback)
	s.Equal(source.URL("invalid/name", ""), fallback)
	s.Equal(source.URL("theme-name", path), fallback)
	s.Equal(source.URL("", "relative.png"), fallback)
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	var data bytes.Buffer
	s.Require().NoError(png.Encode(&data, img))
	s.Require().NoError(os.WriteFile(path, data.Bytes(), 0o600))
	s.Equal(fallback, source.URL("", path), "negative lookup stays cached until refresh")
	source.Refresh(false)
	loaded := source.URL("", path)
	s.NotEqual(fallback, loaded)
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(loaded, "data:image/png;base64,"))
	s.Require().NoError(err)
	actual, err := png.Decode(bytes.NewReader(decoded))
	s.Require().NoError(err)
	s.Equal(color.NRGBAModel.Convert(img.At(0, 0)), color.NRGBAModel.Convert(actual.At(0, 0)))
	lookups := source.Stats().Lookups
	for range 20 {
		s.Equal(loaded, source.URL("", path))
	}
	s.Equal(lookups, source.Stats().Lookups, "cache hits do not resolve paths again")
	for n := range Capacity + 10 {
		source.URL(fmt.Sprintf("missing-icon-%d", n), "")
	}
	s.Equal(Capacity, source.Stats().Entries)
	s.LessOrEqual(source.Stats().Bytes, Capacity*len(fallback))
	before := source.Stats().Lookups
	s.Equal(loaded, source.URL("", path))
	s.Equal(before+1, source.Stats().Lookups, "evicted source is resolved again")
	runtime.GC()
}
