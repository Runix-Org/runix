package product

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func (s *geometrySuite) TestPlacementNormalizesBeforeMeasuring() {
	interpreter, err := exec.LookPath("lua")
	if err != nil {
		s.T().Skip("Lua interpreter is required for isolated Hyprland geometry tests")
	}
	for _, mode := range []int{0, 1, 2} {
		s.Run(fmt.Sprintf("fullscreen-%d", mode), func() {
			directory := s.T().TempDir()
			state := filepath.Join(directory, "state")
			s.T().Setenv("RUNIX_GEOMETRY_STATE", state)
			s.T().Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
			initial := fmt.Sprintf("%d 0 %d %d 1920 1080 0 0", os.Getpid(), mode, mode)
			s.Require().NoError(os.WriteFile(state, []byte(initial), 0o600))
			// #nosec G306 -- this owner-only fixture must be executable as the fake hyprctl.
			s.Require().NoError(os.WriteFile(filepath.Join(directory, "hyprctl"),
				[]byte("#!"+interpreter+"\n"+geometryScenario), 0o700))
			readQt := func(context.Context) ([2]int, error) {
				data, readErr := os.ReadFile(state)
				if readErr != nil {
					return [2]int{}, readErr
				}
				var pid, floating, internal, client, width, height, x, y int
				_, readErr = fmt.Fscan(strings.NewReader(string(data)),
					&pid, &floating, &internal, &client, &width, &height, &x, &y)
				if readErr != nil {
					return [2]int{}, readErr
				}
				if floating != 1 || internal != 0 || client != 0 {
					return [2]int{}, fmt.Errorf("Qt size sampled before window normalization")
				}
				return [2]int{width * 4 / 5, height * 4 / 5}, nil
			}
			saved := &usage.WindowGeometry{X: 140, Y: 180, Positioned: true}
			request := geometryRequest{Width: 608, Height: 448}
			for range 2 {
				s.Require().NoError(placeGeometry(context.Background(), request, saved, readQt))
				data, readErr := os.ReadFile(state)
				s.Require().NoError(readErr)
				s.Equal(fmt.Sprintf("%d 1 0 0 760 560 140 180", os.Getpid()), string(data))
			}
		})
	}
}

// Execute the real Lua requests against isolated native-window state. A fullscreen
// window has different dimensions; sampling it would produce the wrong restored size.
const geometryScenario = `
local path = os.getenv("RUNIX_GEOMETRY_STATE")
local file = assert(io.open(path))
local pid, floating, internal, client, width, height, x, y = file:read("*n", "*n", "*n", "*n", "*n", "*n", "*n", "*n")
file:close()
local w = {mapped = true}
if arg[1] == "-j" and arg[2] == "clients" then
    print(string.format('[{"pid":%d,"mapped":true,"floating":%s,"fullscreen":%d,' ..
        '"fullscreenClient":%d,"size":[%d,%d],"at":[%d,%d],"monitor":1}]',
        pid, tostring(floating == 1), internal, client, width, height, x, y))
elseif arg[1] == "-j" and arg[2] == "monitors" then
    print('[{"id":1,"width":2000,"height":1500,"scale":1.25}]')
elseif arg[1] == "eval" then
    local function dispatch(kind)
        return function(options) options.kind = kind; return options end
    end
    hl = {
        get_window = function(selector) assert(selector == "pid:" .. pid); return w end,
        dsp = {window = {fullscreen_state = dispatch("fullscreen"), float = dispatch("float"),
            resize = dispatch("resize"), move = dispatch("move")}},
        dispatch = function(options)
            assert(options.window == w)
            if options.kind == "fullscreen" then
                assert(options.action == "set" and options.layout_aware == false)
                if internal ~= 0 or client ~= 0 then width, height = 800, 525 end
                internal, client = options.internal, options.client
            elseif options.kind == "float" then
                if floating == 0 then width, height = 800, 525 end
                if options.action == "enable" or options.action == "on" then floating = 1
                elseif options.action == "disable" or options.action == "off" then floating = 0
                else floating = 1 - floating end
            else
                assert(internal == 0 and client == 0 and floating == 1, "Window is fullscreen")
                if options.kind == "resize" then width, height = options.x, options.y
                else x, y = options.x, options.y end
            end
        end,
    }
    assert(load(arg[2]))()
    file = assert(io.open(path, "w"))
    file:write(string.format("%d %d %d %d %d %d %d %d", pid, floating, internal, client, width, height, x, y))
    file:close()
    print("ok")
else error("Unexpected hyprctl request") end
`
