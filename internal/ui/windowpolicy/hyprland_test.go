package windowpolicy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/qml"
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

func (s *SizeSuite) TestWaitsForOwnMappedWindow() {
	directory := s.T().TempDir()
	helper := filepath.Join(directory, "hyprctl")
	pending := filepath.Join(directory, "pending")
	mapped := filepath.Join(directory, "mapped")
	script := fmt.Sprintf("#!/bin/sh\n"+
		"if [ ! -e %q ]; then\n"+
		"  touch %q\n"+
		"  printf '[{\"pid\":%d,\"mapped\":false},{\"pid\":0,\"mapped\":true}]'\n"+
		"else\n"+
		"  touch %q\n"+
		"  printf '[{\"pid\":%d,\"mapped\":true}]'\n"+
		"fi\n", pending, pending, os.Getpid(), mapped, os.Getpid())
	s.Require().NoError(os.WriteFile(helper, []byte(script), 0o600))
	s.Require().NoError(os.Chmod(helper, 0o700))
	s.T().Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s.Require().NoError(waitForMapping(ctx))
	_, err := os.Stat(mapped)
	s.NoError(err, "must not accept an unmapped window or another process's mapped window")
}

func (s *SizeSuite) TestCompositorOwnsScreenClampWithoutLimitingInteractiveSize() {
	s.T().Setenv("QT_QPA_PLATFORM", "offscreen")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app := qt.NewQApplication([]string{"runix-policy-test"})
	defer app.Delete()
	engine := qml.NewQQmlApplicationEngine()
	defer engine.Delete()
	s.Require().NoError(Install(engine))
	engine.LoadData([]byte(`import QtQuick
import QtQuick.Window
Window {
    Component.onCompleted: {
        windowPolicy.compositorPlacement = true
        windowPolicy.savedSize = Qt.size(1600, 1200)
        windowPolicy.attach(this)
    }
}`))
	s.Require().Len(engine.RootObjects(), 1)
	root := engine.RootObjects()[0]
	check := func(source string) {
		expression := qml.NewQQmlExpression2(engine.RootContext(), root, source)
		defer expression.Delete()
		value := expression.Evaluate()
		runtime.SetFinalizer(value, nil)
		defer value.Delete()
		s.Require().False(expression.HasError())
		s.True(value.ToBool(), source)
	}
	check("width === 1600 && height === 1200 && maximumWidth > 1600 && maximumHeight > 1200")
	check(`windowPolicy.compositorPlacement = false;
        width <= windowPolicy.availableWidth && height <= windowPolicy.availableHeight`)
}
