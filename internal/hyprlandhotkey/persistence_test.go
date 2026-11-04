package hyprlandhotkey

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"runix/data/hyprland"
)

func (s *moduleSuite) TestPersistentRemovalAndProductionInvocation() {
	interpreter, err := exec.LookPath("lua")
	if err != nil {
		s.T().Skip("Lua interpreter is required for isolated module tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	path := filepath.Join(s.T().TempDir(), "runix.lua")
	s.Require().NoError(os.WriteFile(path, []byte(hyprland.Source), 0o600))
	command := exec.CommandContext(ctx, interpreter, "-", path)
	command.Stdin = strings.NewReader(persistentModuleScenario)
	var diagnostics bytes.Buffer
	command.Stderr = &diagnostics
	s.Require().NoError(command.Run(), diagnostics.String())
}

const persistentModuleScenario = `
local timers, events, commands, objects = {}, {}, {}, {}
hl = {
    dsp = {},
    on = function(event, callback) events[event] = callback; return {} end,
    exec_cmd = function(command) commands[#commands + 1] = command end,
    timer = function(callback, options)
        timers[#timers + 1] = {callback = callback, options = options}; return {}
    end,
    bind = function(_, callback)
        local object = { enabled = true, alive = true, arg = tostring(#objects + 1), callback = callback }
        function object:is_enabled() if self.alive then return self.enabled end end
        function object:set_enabled(value)
            if self.fail_disable and not value then error("disable failed") end
            self.enabled = value
        end
        function object:remove() self.alive = false end
        objects[#objects + 1] = object
        return object
    end,
}
local executable = "/tmp/runix'; touch /tmp/never-execute; '"
local module = loadfile(arg[1])({ executable = executable })
local function request(op, handle)
    return module.request({version = 1, op = op, epoch = module.epoch,
        revision = module.revision, handle = handle or "", keycode = 38, modifiers = 77})
end
assert(#commands == 0)
events["config.reloaded"]()
assert(module.ready and #commands == 1)
assert(commands[1]:find("hotkeys restore --epoch", 1, true))
assert(commands[1]:find("'\\''", 1, true), "executable must be shell quoted")
request("create")
local handle = module.pending.new_handle
request("commit", handle)
objects[1].callback()
assert(commands[2]:find("cmd full", 1, true))
assert(commands[2]:find("hyprctl notify", 1, true))
local count = #commands
for _, timer in ipairs(timers) do timer.callback() end
assert(#commands == count, "completed transactions must not launch recovery helpers")
request("suspend", handle)
assert(module.pending.removal and not objects[1].enabled)
request("rollback", handle)
assert(module.pending == nil and objects[1].alive and objects[1].enabled)
objects[1].fail_disable = true
request("suspend", handle)
assert(module.pending.removal, "partial removal failure must retain rollback state")
request("rollback", handle)
assert(module.pending == nil and objects[1].enabled)
objects[1].fail_disable = false
request("suspend", handle)
local recovery = timers[#timers]
assert(recovery.options.type == "oneshot")
recovery.callback()
assert(#commands == count + 1, "pending transactions launch one finite recovery helper")
request("commit", handle)
assert(module.pending == nil and not objects[1].alive and next(module.bindings) == nil)
`
