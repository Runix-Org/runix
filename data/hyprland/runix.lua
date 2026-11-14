local INTEGRATION_ID = "__RUNIX_INTEGRATION_ID__"

local PROTOCOL_VERSION = 1
local MAX_BINDINGS = 32
local MODIFIERS = {
    { bit = 1, name = "SHIFT" },
    { bit = 4, name = "CTRL" },
    { bit = 8, name = "ALT" },
    { bit = 64, name = "SUPER" },
}

local function json_string(value)
    return '"' .. value:gsub('[%z\1-\31\\"]', function(char)
        local escapes = {
            ['"'] = '\\"', ['\\'] = '\\\\', ['\b'] = '\\b',
            ['\f'] = '\\f', ['\n'] = '\\n', ['\r'] = '\\r', ['\t'] = '\\t',
        }
        return escapes[char] or string.format('\\u%04x', char:byte())
    end) .. '"'
end

local function encode(value)
    local kind = type(value)
    if kind == "nil" then return "null" end
    if kind == "boolean" or kind == "number" then return tostring(value) end
    if kind == "string" then return json_string(value) end
    if kind ~= "table" then error("unsupported JSON value") end

    local parts = {}
    local metatable = getmetatable(value)
    if #value > 0 or (metatable and metatable.__json_array) then
        for index = 1, #value do parts[index] = encode(value[index]) end
        return "[" .. table.concat(parts, ",") .. "]"
    end
    for key, item in pairs(value) do
        parts[#parts + 1] = json_string(key) .. ":" .. encode(item)
    end
    return "{" .. table.concat(parts, ",") .. "}"
end

local function epoch_value()
    return tostring(os.time()) .. "-" .. tostring(os.clock()) .. "-" .. tostring({})
end

local function safe_call(fn, ...)
    local ok, result = pcall(fn, ...)
    if not ok then return false, tostring(result) end
    return true, result
end

local function shell_quote(value)
    return "'" .. value:gsub("'", "'\\''") .. "'"
end

local function initialize(options)
    options = options or {}
    if type(options) ~= "table" then error("options must be a table") end
    if rawget(_G, "runix_hotkeys") and _G.runix_hotkeys._active then
        error("runix_hotkeys is already active")
    end
    local hl = rawget(_G, "hl")
    if type(hl) ~= "table" or type(hl.bind) ~= "function" or type(hl.dsp) ~= "table" then
        error("Hyprland Lua binding API is unavailable")
    end
    if options.executable ~= nil and (type(options.executable) ~= "string" or options.executable == "") then
        error("executable must be a non-empty string")
    end
    if options.on_toggle ~= nil and type(options.on_toggle) ~= "function" then
        error("on_toggle must be a function")
    end
    if options.on_ready ~= nil and type(options.on_ready) ~= "function" then
        error("on_ready must be a function")
    end

    hl.window_rule({
        name = "runix-window",
        match = { class = "^runix$" },
        float = true,
        fullscreen_state = "0 0",
        suppress_event = "fullscreen maximize",
    })

    local module = {
        _active = true,
        epoch = epoch_value(),
        revision = 1,
        executable = options.executable or "runix",
        bindings = {},
        pending = nil,
        ready = false,
        next_handle = 0,
    }
    _G.runix_hotkeys = module

    local function notify(message)
        if hl.notification and hl.notification.create then
            pcall(hl.notification.create, { text = message, timeout = 8000, icon = "error" })
        end
    end

    local function spawn(arguments)
        local command = shell_quote(module.executable) .. " " .. arguments
        command = command .. " || hyprctl notify 3 8000 0 " ..
            shell_quote("Runix command failed. Check the executable and run runix hotkeys status.")
        local ok, result = safe_call(hl.exec_cmd, command)
        if not ok then
            notify("Could not start runix: " .. tostring(result))
            error(result)
        end
    end

    options.on_toggle = options.on_toggle or function() spawn("cmd full") end
    options.on_ready = options.on_ready or function(epoch)
        spawn("hotkeys restore --epoch " .. shell_quote(epoch))
    end

    local function schedule_recovery()
        local pending = module.pending
        local ok, timer = safe_call(hl.timer, function()
            if module._active and module.pending == pending then
                local launched, err = pcall(spawn, "hotkeys restore --epoch " .. shell_quote(module.epoch))
                if not launched then notify("Runix hotkey recovery failed: " .. tostring(err)) end
            end
        end, { timeout = 2500, type = "oneshot" })
        if ok and timer then
            module.recovery_timer = timer
        else
            notify("Could not schedule runix hotkey recovery.")
        end
    end

    local function error_object(code, message)
        return { code = code, message = message }
    end

    local function state(error)
        local bindings = setmetatable({}, { __json_array = true })
        for handle, record in pairs(module.bindings) do
            local ok, enabled = safe_call(record.object.is_enabled, record.object)
            if not ok or enabled == nil then
                if module.pending and (module.pending.new_handle == handle or module.pending.old_handle == handle) then
                    error = error or error_object("runtime_error", "A pending binding handle was removed externally")
                else
                    module.bindings[handle] = nil
                end
            else
                record.enabled = enabled == true
                bindings[#bindings + 1] = {
                    handle = handle, arg = record.arg, keycode = record.keycode,
                    modifiers = record.modifiers, enabled = record.enabled, action = "toggle",
                }
            end
        end
        if not error and module.lifecycle_error then error = error_object("runtime_error", module.lifecycle_error) end
        return {
            integration_id = INTEGRATION_ID, version = PROTOCOL_VERSION, epoch = module.epoch, revision = module.revision,
            ready = module.ready, executable = module.executable, path = os.getenv("PATH") or "",
            toggle_available = type(options.on_toggle) == "function", bindings = bindings,
            pending = module.pending and module.pending.new_handle or "", error = error,
            pending_removal = module.pending and module.pending.removal == true or false,
        }
    end

    local function remove_record(handle)
        local record = module.bindings[handle]
        if not record then return nil, "owned handle no longer exists" end
        local ok, enabled = safe_call(record.object.is_enabled, record.object)
        if not ok then return nil, enabled end
        if enabled == nil then return nil, "owned handle was removed externally" end
        if enabled then
            local disabled, disable_error = safe_call(record.object.set_enabled, record.object, false)
            if not disabled then return nil, disable_error end
            local verified, current = safe_call(record.object.is_enabled, record.object)
            if not verified or current ~= false then return nil, "could not verify disabled binding state" end
        end
        local removed, remove_error = safe_call(record.object.remove, record.object)
        if not removed then return nil, remove_error end
        local verified, current = safe_call(record.object.is_enabled, record.object)
        if not verified or current ~= nil then return nil, "could not verify binding removal" end
        module.bindings[handle] = nil
        return true
    end

    local function respond(error)
        local ok, result = pcall(encode, state(error))
        if ok then return result end
        return encode({ integration_id = INTEGRATION_ID, version = PROTOCOL_VERSION, epoch = module.epoch, revision = module.revision,
            ready = false, executable = module.executable, path = os.getenv("PATH") or "",
            toggle_available = false, bindings = setmetatable({}, { __json_array = true }), pending = "",
            error = error_object("runtime_error", "Could not encode module state: " .. tostring(result)) })
    end

    local function check(request, mutation)
        if type(request) ~= "table" then return error_object("invalid_request", "Request must be an object") end
        if request.version ~= PROTOCOL_VERSION then return error_object("unsupported_version", "Unsupported protocol version") end
        local allowed = { version = true, op = true, epoch = true, revision = true, handle = true, keycode = true, modifiers = true }
        for key in pairs(request) do
            if not allowed[key] then return error_object("invalid_request", "Request contains an unsupported field") end
        end
        if type(request.op) ~= "string" then return error_object("invalid_request", "op must be a string") end
        if request.op ~= "status" and request.op ~= "create" and request.op ~= "replace"
            and request.op ~= "remove" and request.op ~= "suspend" and request.op ~= "rollback" and request.op ~= "commit" then
            return error_object("invalid_request", "Unknown operation")
        end
        local needs_handle = request.op ~= "status" and request.op ~= "create"
        if needs_handle or request.handle ~= nil then
            if type(request.handle) ~= "string" or #request.handle > 256
                or (needs_handle and #request.handle == 0) then
                return error_object("invalid_request", "handle must identify the owned binding or transaction")
            end
        end
        if mutation then
            if not module.ready then return error_object("not_ready", "Hotkey module is not ready") end
            if type(request.epoch) ~= "string" or request.epoch ~= module.epoch then return error_object("stale_epoch", "Request belongs to an old module lifetime") end
            if type(request.revision) ~= "number" or request.revision % 1 ~= 0 or request.revision ~= module.revision then return error_object("stale_revision", "Request uses an old state revision") end
        end
        return nil
    end


    local function create_binding(request)
        if type(request.keycode) ~= "number" or request.keycode % 1 ~= 0 or request.keycode < 9 or request.keycode > 255 then
            return nil, error_object("invalid_request", "keycode must be an integer from 9 through 255")
        end
        if type(request.modifiers) ~= "number" or request.modifiers % 1 ~= 0 or request.modifiers < 0 then
            return nil, error_object("invalid_request", "modifiers must be a non-negative integer mask")
        end
        local rest = request.modifiers
        for _, modifier in ipairs(MODIFIERS) do
            if math.floor(rest / modifier.bit) % 2 == 1 then rest = rest - modifier.bit end
        end
        if rest ~= 0 then return nil, error_object("invalid_request", "modifiers contain unsupported bits") end
        if type(options.on_toggle) ~= "function" then
            return nil, error_object("missing_handler", "No toggle handler is configured")
        end
        local parts = {}
        for _, modifier in ipairs(MODIFIERS) do
            if math.floor(request.modifiers / modifier.bit) % 2 == 1 then parts[#parts + 1] = modifier.name end
        end
        parts[#parts + 1] = "code:" .. request.keycode
        module.next_handle = module.next_handle + 1
        local handle = module.epoch .. ":" .. tostring(module.next_handle)
        local dispatcher = function()
            local ok, err = pcall(options.on_toggle, module.executable)
            if not ok then error(err) end
        end
        local bound, object = safe_call(hl.bind, table.concat(parts, " + "), dispatcher, {
            description = "Runix hotkey", repeating = false, locked = false,
        })
        if not bound then return nil, error_object("runtime_error", "Could not create binding: " .. tostring(object)) end
        if object == nil or type(object.remove) ~= "function" or type(object.is_enabled) ~= "function" then
            return nil, error_object("runtime_error", "Hyprland returned an unsupported binding handle")
        end
        if type(object.arg) ~= "string" then
            safe_call(object.remove, object)
            return nil, error_object("runtime_error", "Hyprland did not expose the binding action identity")
        end
        module.bindings[handle] = { object = object, arg = object.arg, keycode = request.keycode, modifiers = request.modifiers }
        return handle
    end

    module.request = function(request)
        local op = type(request) == "table" and request.op or nil
        local error = check(request, op ~= "status")
        if error then return respond(error) end
        if op == "status" then return respond(nil) end
        if module.pending and op ~= "rollback" and op ~= "commit" then
            return respond(error_object("pending", "A binding transaction is already pending"))
        end
        local binding_count = 0
        for _ in pairs(module.bindings) do binding_count = binding_count + 1 end
        if op == "create" then
            if binding_count >= MAX_BINDINGS then return respond(error_object("runtime_error", "Owned binding limit reached")) end
            local handle, create_error = create_binding(request)
            if not handle then return respond(create_error) end
            module.pending = { new_handle = handle, old_handle = nil }
            module.revision = module.revision + 1
            schedule_recovery()
        elseif op == "replace" then
            local old = module.bindings[request.handle]
            if not old then return respond(error_object("missing_handle", "Owned binding handle was not found")) end
            if binding_count >= MAX_BINDINGS then return respond(error_object("runtime_error", "Owned binding limit reached")) end
            local handle, create_error = create_binding(request)
            if not handle then return respond(create_error) end
            module.pending = { new_handle = handle, old_handle = request.handle }
            module.revision = module.revision + 1
            schedule_recovery()
            local disabled, disable_error = safe_call(old.object.set_enabled, old.object, false)
            if not disabled then return respond(error_object("runtime_error", "Could not disable old binding: " .. tostring(disable_error))) end
            local verified, current = safe_call(old.object.is_enabled, old.object)
            if not verified or current ~= false then
                return respond(error_object("runtime_error", "Could not verify old binding is disabled"))
            end
        elseif op == "suspend" then
            local old = module.bindings[request.handle]
            if not old then return respond(error_object("missing_handle", "Owned binding handle was not found")) end
            module.pending = { new_handle = request.handle, old_handle = request.handle, removal = true }
            module.revision = module.revision + 1
            schedule_recovery()
            local disabled, disable_error = safe_call(old.object.set_enabled, old.object, false)
            if not disabled then return respond(error_object("runtime_error", "Could not disable binding: " .. tostring(disable_error))) end
            local verified, current = safe_call(old.object.is_enabled, old.object)
            if not verified or current ~= false then
                return respond(error_object("runtime_error", "Could not verify disabled binding"))
            end
        elseif op == "remove" then
            if not module.bindings[request.handle] then return respond(error_object("missing_handle", "Owned binding handle was not found")) end
            local removed, remove_error = remove_record(request.handle)
            if not removed then return respond(error_object("runtime_error", "Could not remove binding: " .. tostring(remove_error))) end
            module.revision = module.revision + 1
        elseif op == "rollback" then
            if not module.pending then return respond(error_object("invalid_request", "There is no pending transaction")) end
            local pending = module.pending
            if request.handle ~= pending.new_handle then return respond(error_object("invalid_request", "Handle does not match pending transaction")) end
            local new = module.bindings[pending.new_handle]
            if new and not pending.removal then
                local valid, enabled = safe_call(new.object.is_enabled, new.object)
                if not valid then return respond(error_object("runtime_error", "Could not inspect new binding: " .. tostring(enabled))) end
                if enabled ~= nil then
                    local removed, remove_error = remove_record(pending.new_handle)
                    if not removed then return respond(error_object("runtime_error", "Rollback could not remove new binding: " .. tostring(remove_error))) end
                else
                    module.bindings[pending.new_handle] = nil
                end
            end
            if pending.old_handle then
                local old = module.bindings[pending.old_handle]
                if not old then return respond(error_object("runtime_error", "Rollback cannot restore missing old binding")) end
                local valid, enabled = safe_call(old.object.is_enabled, old.object)
                if not valid or enabled == nil then return respond(error_object("runtime_error", "Rollback cannot restore removed old binding")) end
                local restored, restore_error = safe_call(old.object.set_enabled, old.object, true)
                if not restored then return respond(error_object("runtime_error", "Rollback could not restore old binding: " .. tostring(restore_error))) end
                local verified, current = safe_call(old.object.is_enabled, old.object)
                if not verified or current ~= true then return respond(error_object("runtime_error", "Could not verify restored old binding")) end
            end
            module.pending = nil
            module.revision = module.revision + 1
        elseif op == "commit" then
            if not module.pending then return respond(error_object("invalid_request", "There is no pending transaction")) end
            local pending = module.pending
            if request.handle ~= pending.new_handle then return respond(error_object("invalid_request", "Handle does not match pending transaction")) end
            local new = module.bindings[pending.new_handle]
            if not new then return respond(error_object("runtime_error", "Commit cannot verify missing new binding")) end
            local valid, enabled = safe_call(new.object.is_enabled, new.object)
            if not valid or enabled ~= (not pending.removal) then
                return respond(error_object("runtime_error", "Commit cannot verify pending binding state"))
            end
            if pending.old_handle then
                local old = module.bindings[pending.old_handle]
                if not old then return respond(error_object("runtime_error", "Commit cannot verify missing old binding")) end
                local old_valid, old_enabled = safe_call(old.object.is_enabled, old.object)
                if not old_valid or old_enabled ~= false then return respond(error_object("runtime_error", "Commit cannot verify disabled old binding")) end
                local removed, remove_error = remove_record(pending.old_handle)
                if not removed then return respond(error_object("runtime_error", "Commit could not remove old binding: " .. tostring(remove_error))) end
            end
            module.pending = nil
            module.revision = module.revision + 1
        else
            return respond(error_object("invalid_request", "Unknown operation"))
        end
        return respond(nil)
    end

    local function ready()
        if not module._active or module.ready or module.lifecycle_error then return end
        module.ready = true
        if type(options.on_ready) == "function" then
            local ok, err = pcall(options.on_ready, module.epoch)
            if not ok then
                module.ready = false
                module.lifecycle_error = "on_ready failed: " .. tostring(err)
            end
        end
    end

    local function unload()
        module.ready = false
        module._active = false
        module.pending = nil
        module.bindings = {}
    end

    if type(hl.on) ~= "function" or type(hl.timer) ~= "function" then
        module.lifecycle_error = "Hyprland event and timer APIs are unavailable"
    else
        module.subscriptions = {}
        for _, event in ipairs({ "config.reloaded", "config.unload", "hyprland.start" }) do
            local callback = event == "config.unload" and unload or ready
            local ok, subscription = safe_call(hl.on, event, callback)
            if not ok or subscription == nil then
                module.lifecycle_error = "Could not subscribe to " .. event .. ": " .. tostring(subscription)
                break
            end
            module.subscriptions[#module.subscriptions + 1] = subscription
        end
        if not module.lifecycle_error then
            local ok, timer = safe_call(hl.timer, ready, { timeout = 1, type = "oneshot" })
            if not ok or timer == nil then
                module.lifecycle_error = "Could not schedule setup readiness: " .. tostring(timer)
            else
                module.timer = timer
            end
        end
    end
    if module.lifecycle_error then module.ready = false end
    return module
end
local options = ...
if type(options) == "string" then options = nil end -- require passes the module name.
return initialize(options)
