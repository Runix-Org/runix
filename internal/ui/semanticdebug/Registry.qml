import QtQuick

QtObject {
    id: registry
    property var window: null
    property var dismiss: null
    property var controls: ({})

    function registerControl(id, item, role, label, actions, activate, secret) {
        controls[id] = {item: item, role: role, label: label, actions: actions,
                        activate: activate, secret: secret === true}
    }

    function fail(code, message) {
        return {error: {code: code, message: message}}
    }

    function describe(id, entry) {
        const item = entry.item
        if (!item)
            return {id: id, destroyed: true, actions: []}
        const origin = item.mapToItem(window.contentItem, 0, 0)
        const bounds = {x: origin.x, y: origin.y, width: item.width, height: item.height}
        let left = Math.max(0, origin.x)
        let top = Math.max(0, origin.y)
        let right = Math.min(window.width, origin.x + item.width)
        let bottom = Math.min(window.height, origin.y + item.height)
        for (let parent = item.parent; parent; parent = parent.parent) {
            if (parent.clip) {
                const p = parent.mapToItem(window.contentItem, 0, 0)
                left = Math.max(left, p.x)
                top = Math.max(top, p.y)
                right = Math.min(right, p.x + parent.width)
                bottom = Math.min(bottom, p.y + parent.height)
            }
        }
        const visible = window.visible && item.visible && item.opacity > 0
        const inView = visible && right > left && bottom > top
        const description = {id: id, role: entry.role, label: entry.label, visible: visible,
                enabled: item.enabled, focused: item.activeFocus,
                value: entry.secret ? null : String(item.text === undefined ? "" : item.text),
                redacted: entry.secret, actions: entry.actions, bounds: bounds,
                in_view: inView, clipped: !inView || left > origin.x || top > origin.y
                    || right < origin.x + item.width || bottom < origin.y + item.height,
                visible_bounds: {x: left, y: top, width: Math.max(0, right-left),
                                 height: Math.max(0, bottom-top)}}
        if (!entry.secret && entry.inspect) description.model = entry.inspect()
        return description
    }

    function dispatch(request) {
        if (!window)
            return fail("destroyed", "The laboratory window is unavailable")
        if (request.op === "snapshot") {
            const items = []
            for (const id of Object.keys(controls))
                items.push(describe(id, controls[id]))
            return {result: {window: {id: "ui-lab", title: window.title, visible: window.visible,
                                     active: window.active, width: window.width, height: window.height,
                                     screen: window.Screen.name, screen_dpr: window.Screen.devicePixelRatio,
                                     coordinate_space: "window-local-logical"}, controls: items}}
        }
        if (!Object.prototype.hasOwnProperty.call(controls, request.target))
            return fail("unknown_id", "No control has this semantic ID")
        const entry = controls[request.target]
        const item = entry.item
        if (!item)
            return fail("destroyed", "The control has been destroyed")
        if (!item.enabled)
            return fail("disabled", "The control is disabled")
        if (!window.visible || !item.visible)
            return fail("hidden", "The control is hidden")
        if (entry.actions.indexOf(request.action) < 0)
            return fail("unsupported_action", "The control does not support this action")
        switch (request.action) {
        case "focus":
            item.forceActiveFocus(Qt.OtherFocusReason)
            break
        case "replace":
            item.remove(0, item.length)
            item.insert(0, request.text || "")
            break
        case "select":
            if (entry.select) {
                if (!entry.select(request.text || ""))
                    return fail("invalid_selection", "Choose an available result index")
            } else {
                if (request.text) return fail("invalid_selection", "Text selection takes no argument")
                item.selectAll()
            }
            break
        case "scroll":
            if (!entry.scroll || !entry.scroll(request.text || ""))
                return fail("invalid_scroll", "Choose an available result index")
            break
        case "activate":
            entry.activate()
            break
        case "key":
            if (!item.activeFocus)
                return fail("not_focused", "Focus the control before a keyboard action")
            switch (request.text) {
            case "Enter":
                if (entry.role === "textbox") item.accepted()
                else if (entry.activate) entry.activate()
                else return fail("unsupported_key", "This control does not support Enter")
                break
            case "Space":
                if (!entry.activate)
                    return fail("unsupported_key", "This control does not support Space")
                entry.activate()
                break
            case "Tab":
            case "Backtab":
                item.nextItemInFocusChain(request.text === "Tab").forceActiveFocus(Qt.TabFocusReason)
                break
            case "Escape":
                if (typeof dismiss !== "function")
                    return fail("unsupported_key", "The window has no dismiss handler")
                dismiss()
                break
            default:
                return fail("unsupported_key", "This keyboard action is not supported")
            }
            break
        default:
            return fail("unsupported_action", "This action is not supported")
        }
        return {result: {classification: "semantic", target: request.target, action: request.action}}
    }
}
