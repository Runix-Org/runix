package product

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"runix/internal/usage"
)

type geometrySuite struct{ suite.Suite }

func TestGeometrySuite(t *testing.T) { suite.Run(t, new(geometrySuite)) }

func (s *geometrySuite) TestRestoreClampsToUsableOutput() {
	monitor := geometryMonitor{
		ID: 1, X: -1600, Width: 2000, Height: 1500, Scale: 1.25,
		Reserved: [4]int{0, 40, 0, 0},
	}
	saved := &usage.WindowGeometry{X: -100, Y: 1000, Positioned: true}
	x, y, width, height := restoredBounds(saved, []geometryMonitor{monitor}, monitor, 800, 640)
	s.Equal(-800, x)
	s.Equal(560, y)
	s.Equal(800, width)
	s.Equal(640, height)
}

func (s *geometrySuite) TestDisconnectedOutputFallsBackAndOversizedWindowFits() {
	monitor := geometryMonitor{
		ID: 1, Width: 1080, Height: 1920, Transform: 1, Scale: 2,
		Reserved: [4]int{0, 20, 0, 0},
	}
	saved := &usage.WindowGeometry{X: -2000, Y: 100, Positioned: true}
	x, y, width, height := restoredBounds(saved, []geometryMonitor{monitor}, monitor, 1200, 900)
	s.Equal(0, x)
	s.Equal(20, y)
	s.Equal(960, width)
	s.Equal(520, height)
}
