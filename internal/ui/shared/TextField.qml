import QtQuick
import QtQuick.Controls

TextField {
    id: control
    property bool search: false
    property string errorText: ""
    Accessible.description: errorText
    color: runixControlStyle.foreground(control)
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
    implicitHeight: Math.max(runixControlStyle.fieldHeight, contentHeight + topPadding + bottomPadding)
    padding: search ? 9 : 6
    leftPadding: search ? 11 : runixControlStyle.spacing
    rightPadding: leftPadding
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
            color: control.readOnly || control.search ? "transparent" : runixControlStyle.surface
            border.width: control.search ? 0 : 1
            border.color: !control.enabled ? runixControlStyle.disabledBorder
                : control.errorText.length > 0 ? runixControlStyle.error
                    : control.hovered && !control.readOnly
                        ? runixControlStyle.hoverBorder : runixControlStyle.fieldBorder
            Rectangle {
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.bottom: parent.bottom
                height: 1
                color: control.search ? runixControlStyle.separator : "transparent"
            }
        }
    }
}
