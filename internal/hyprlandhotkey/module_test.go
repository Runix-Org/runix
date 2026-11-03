package hyprlandhotkey

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"runix/data/hyprland"
)

type moduleSuite struct{ suite.Suite }

func TestModuleSuite(t *testing.T) {
	suite.Run(t, new(moduleSuite))
}

func (s *moduleSuite) TestPartialRollbackCanBeRetriedWithoutLosingOldBinding() {
	interpreter, err := exec.LookPath("lua")
	if err != nil {
		s.T().Skip("Lua interpreter is required for isolated module failure tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	modulePath := filepath.Join(s.T().TempDir(), "runix.lua")
	s.Require().NoError(os.WriteFile(modulePath, []byte(hyprland.Source), 0o600))
	command := exec.CommandContext(ctx, interpreter, "-", modulePath)
	command.Stdin = strings.NewReader(moduleFailureScenario)
	var output, diagnostics bytes.Buffer
	command.Stdout, command.Stderr = &output, &diagnostics
	s.Require().NoError(command.Run(), diagnostics.String())
	var snapshots []State
	decoder := json.NewDecoder(&output)
	for decoder.More() {
		var state State
		s.Require().NoError(decoder.Decode(&state))
		snapshots = append(snapshots, state)
	}
	s.Require().Len(snapshots, 9)
	s.False(snapshots[0].Ready, "setup must not advertise readiness inside configuration evaluation")
	s.True(snapshots[1].Ready)
	s.Equal(hyprland.Identity, snapshots[1].IntegrationID, "the deployed source must report its stamped identity")
	s.Require().NotNil(snapshots[2].Error)
	s.Equal("invalid_request", snapshots[2].Error.Code)
	s.Empty(snapshots[2].Pending)
	s.Require().Len(snapshots[2].Bindings, 1)
	s.True(snapshots[2].Bindings[0].Enabled, "invalid replacement must preserve the old binding")
	s.Require().NotNil(snapshots[3].Error)
	s.Equal("invalid_request", snapshots[3].Error.Code)
	s.NotEmpty(snapshots[3].Pending, "wrong transaction handles cannot finalize another client's change")
	s.Require().NotNil(snapshots[4].Error)
	s.Equal("runtime_error", snapshots[4].Error.Code)
	s.NotEmpty(snapshots[4].Pending)
	s.Require().Len(snapshots[4].Bindings, 1)
	s.False(snapshots[4].Bindings[0].Enabled)
	s.Nil(snapshots[5].Error)
	s.Empty(snapshots[5].Pending)
	s.Require().Len(snapshots[5].Bindings, 1)
	s.True(snapshots[5].Bindings[0].Enabled, "retry must restore the retained old object")
	s.Require().NotNil(snapshots[6].Error)
	s.Equal("runtime_error", snapshots[6].Error.Code)
	s.NotEmpty(snapshots[6].Pending, "externally removed new object cannot be committed")
	s.Nil(snapshots[7].Error)
	s.Empty(snapshots[7].Pending)
	s.Require().Len(snapshots[7].Bindings, 1)
	s.True(snapshots[7].Bindings[0].Enabled)
	s.False(snapshots[8].Ready)
	s.Require().NotNil(snapshots[8].Error)
	s.Equal("runtime_error", snapshots[8].Error.Code)
}

// The fake native objects permit deterministic compositor-failure injection. The
// real module executes unchanged; native API/dispatch acceptance is separate.
const moduleFailureScenario = `
local callbacks, objects, deferred = {}, {}, nil
hl = {
    dsp = {},
    exec_cmd = function() end,
    on = function(event, callback) callbacks[event] = callback; return {} end,
    timer = function(callback) deferred = callback; return {} end,
    bind = function()
        local object = { enabled = true, alive = true, arg = tostring(#objects + 1) }
        function object:is_enabled() if self.alive then return self.enabled end end
        function object:set_enabled(value)
            if value and self.fail_restore then error("injected restore failure") end
            if self.alive then self.enabled = value end
        end
        function object:remove() self.alive = false end
        objects[#objects + 1] = object
        return object
    end,
}
local source = dofile(arg[1])
local module = source.setup({on_toggle = function() end})
local function request(op, handle, code)
    return module.request({version = 1, op = op, epoch = module.epoch,
        revision = module.revision, handle = handle or "", keycode = code or 38, modifiers = 64})
end
local function emit(value) io.write(value, "\n") end
emit(request("status"))
deferred()
emit(request("status"))
request("create")
local original = module.pending.new_handle
request("commit", module.pending.new_handle)
emit(request("replace", original, 0))
request("replace", original, 54)
local transaction = module.pending.new_handle
emit(request("commit", "different-transaction"))
objects[1].fail_restore = true
emit(request("rollback", transaction))
objects[1].fail_restore = false
emit(request("rollback", transaction))
request("replace", original, 54)
transaction = module.pending.new_handle
objects[3]:remove()
emit(request("commit", transaction))
emit(request("rollback", transaction))
callbacks["config.unload"]()
module = source.setup({on_ready = function() error("injected lifecycle failure") end})
deferred()
emit(request("status"))
`
