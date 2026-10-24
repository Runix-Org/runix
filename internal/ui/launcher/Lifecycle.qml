import QtQuick

QtObject {
    property var window: null
    property var query: null

    function applyShow() {
        window.show()
        window.requestActivate()
        if (query) {
            query.forceActiveFocus(Qt.ShortcutFocusReason)
        }
        return window.visible
    }

    function applyHide() {
        window.hide()
        return window.visible
    }
}
