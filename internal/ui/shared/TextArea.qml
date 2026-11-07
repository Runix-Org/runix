import QtQuick
import QtQuick.Controls

TextArea {
    id: control
    property string errorText: ""
    color: runixControlStyle.foreground(control)
    Accessible.description: errorText
    placeholderTextColor: enabled ? runixControlStyle.placeholder : runixControlStyle.disabled
    selectByMouse: true
    cursorDelegate: Rectangle {
        width: 1
        height: control.cursorRectangle.height
        color: runixControlStyle.caret
        visible: control.cursorVisible && control.enabled && !control.readOnly
    }
    selectionColor: activeAppearance.surfaces.selection
    selectedTextColor: runixControlStyle.selectedText
    palette.highlight: activeAppearance.surfaces.selection
    palette.highlightedText: runixControlStyle.selectedText
    padding: 10
    background: Item {
        Rectangle {
            anchors.fill: parent
            anchors.margins: -2
            radius: runixControlStyle.radius + 2
            color: "transparent"
            border.width: control.activeFocus ? 2 : 0
            border.color: runixControlStyle.focus
        }
        Rectangle {
            anchors.fill: parent
            radius: runixControlStyle.radius
            color: control.readOnly ? "transparent" : runixControlStyle.surface
            border.width: 1
            border.color: !control.enabled ? runixControlStyle.disabledBorder
                : control.errorText.length > 0 ? runixControlStyle.error
                    : control.hovered && !control.readOnly
                        ? runixControlStyle.hoverBorder : runixControlStyle.fieldBorder
        }
    }
}
