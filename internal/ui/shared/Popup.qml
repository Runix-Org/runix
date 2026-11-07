import QtQuick
import QtQuick.Controls
import QtQuick.Window

Popup {
    id: control
    property bool modalScrim: false
    property Item invokingFocusItem: null
    focus: true
    padding: 12
    Overlay.modal: Rectangle { color: runixControlStyle.scrim }
    modal: modalScrim
    background: Rectangle {
        color: runixControlStyle.popup
        border.color: runixControlStyle.popupBorder
        border.width: 1
        radius: runixControlStyle.radius
    }
    onAboutToShow: invokingFocusItem = Window.activeFocusItem
    onClosed: {
        if (invokingFocusItem) invokingFocusItem.forceActiveFocus()
        invokingFocusItem = null
    }
}
