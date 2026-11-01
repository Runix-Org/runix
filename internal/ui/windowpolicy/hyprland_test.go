package windowpolicy

import (
	"runtime"
	"testing"

	qt "github.com/mappu/miqt/qt6"
	"github.com/stretchr/testify/suite"
)

type SizeSuite struct{ suite.Suite }

func TestSizeSuite(t *testing.T) { suite.Run(t, new(SizeSuite)) }

func (s *SizeSuite) TestRequestsKeepTheirDimensionsAcrossGarbageCollection() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for i := range 40 {
		want := dimensions{640 + i, 420 + i}
		size := qt.NewQSize2(want.width, want.height)
		value := qt.NewQVariant22(size)
		size.Delete()
		got := requestedDimensions(value)
		runtime.GC() // The old conversion deleted its QSize again on the finalizer thread.
		s.Equal(want, got)
		s.Equal(want, requestedDimensions(value))
		value.Delete()
	}
	runtime.GC()
}
