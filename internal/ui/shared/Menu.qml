import QtQuick
import QtQuick.Controls
import QtQuick.Window

Menu {
    id: control
    property Item invokingFocusItem: null
    implicitWidth: 220
    onAboutToShow: invokingFocusItem = Window.activeFocusItem
    onClosed: {
        if (invokingFocusItem) invokingFocusItem.forceActiveFocus()
        invokingFocusItem = null
    }
    background: Rectangle {
        color: runixControlStyle.popup
        border.color: runixControlStyle.popupBorder
        border.width: 1
        radius: runixControlStyle.radius
    }
}
