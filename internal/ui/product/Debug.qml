import QtQuick

Item {
    id: debug
    parent: productDebugWindow.contentItem
    property var window: productDebugWindow
    property int frames: 0
    property int frameRequests: 0

    Connections {
        target: debug.window
        function onFrameSwapped() { debug.frames++ }
    }

    function find(name, root) {
        if (root.objectName === name) return root
        for (const child of root.children || []) {
            const found = find(name, child)
            if (found) return found
        }
        return null
    }

    function control(name) {
        const item = find(name, window.contentItem)
        return item && item.item ? item.item : item
    }

    function register(name, role, label, actions, activate) {
        const item = control(name)
        if (!item) throw new Error("Missing product control: " + name)
        semanticDebug.registerControl(name, item, role, label, actions, activate, false)
        return semanticDebug.controls[name]
    }

    function registerScaleControls() {
        if (!control("settings-interface-scale")) return
        const scale = register("settings-interface-scale", "combobox", "Interface scale", ["focus"])
        scale.readValue = function() { return control("settings-interface-scale").displayText }
        register("settings-scale-apply", "button", "Apply and restart", ["focus", "activate"], function() {
            control("settings-scale-apply").clicked()
        })
    }
    Connections {
        id: appearanceChanges
        target: debug.find("settings-appearance-panel", window.contentItem)
        function onLoaded() { debug.registerScaleControls() }
        function onItemChanged() {
            if (!appearanceChanges.target.item) {
                delete semanticDebug.controls["settings-interface-scale"]
                delete semanticDebug.controls["settings-scale-apply"]
            }
        }
    }

    Component.onCompleted: {
        semanticDebug.window = window
        semanticDebug.windowID = "runix"
        semanticDebug.inspectWindow = function() {
            const results = control("search-results")
            const appearance = control("settings-appearance-panel")
            return {screen: window.integrationOpen ? "integration" : window.onboarding ? "onboarding"
                        : window.settingsOpen ? "settings" : "search",
                    ready: window.integrationStartupResolved && window.shortcutStartupResolved,
                    search: results ? results.inspect() : null,
                    rendering: {frames: frames, frame_swapped: frames > 0, requests: frameRequests,
                                renderer_api: debug.GraphicsInfo.api},
                    settings: {tab: window.settingsTab,
                               interface_scale: appearance && appearance.scaleInfo ? appearance.scaleInfo : null}}
        }
        register("search-query", "textbox", "Search applications and commands", ["focus", "replace", "select"])
        register("integration-continue", "button", "Continue without changing integration", ["focus", "activate"], function() {
            control("integration-continue").clicked()
        })
        const results = register("search-results", "list", "Search results", ["focus", "select"])
        results.inspect = function() { return control("search-results").inspect() }
        results.select = function(text) {
            if (!/^[0-9]+$/.test(text)) return false
            return control("search-results").select(Number(text))
        }
        register("search-primary-action", "button", "Primary action", [])
        register("settings-back", "button", "Back", ["focus", "activate"], function() {
            control("settings-back").clicked()
        })
        register("settings-integration", "button", "Integration", [])
        register("settings-appearance", "button", "Appearance", ["focus", "activate"], function() {
            control("settings-appearance").clicked()
        })
        register("settings-shortcut", "button", "Keyboard shortcut", ["focus", "activate"], function() {
            control("settings-shortcut").clicked()
        })
        register("settings-window-reset", "button", "Reset window layout", ["focus", "activate"], function() {
            control("settings-window-reset").clicked()
        })
        // No generic result activation: development inspection does not authorize external launches.
        semanticDebug.registerControl("settings-open", window.contentItem, "navigation", "Open settings",
                                      ["activate"], null, false)
        semanticDebug.controls["settings-open"].handleAction = function(request) {
            if (request.action !== "activate")
                return semanticDebug.fail("unsupported_action", "This control does not support this action")
            if (!window.openSettingsFromCommand())
                return semanticDebug.fail("unavailable", "Settings navigation is not available")
            return {result: {classification: "semantic", target: request.target, action: request.action}}
        }
        semanticDebug.registerControl("render-frame", window.contentItem, "status", "Request a frame",
                                      ["activate"], function() {
            frameRequests++
            window.update()
        }, false)
    }
}
