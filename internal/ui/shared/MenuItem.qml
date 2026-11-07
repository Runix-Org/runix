import QtQuick
import QtQuick.Controls

MenuItem {
    id: control
    property bool destructive: false
    implicitHeight: 36
    contentItem: Text {
        text: control.text
        textFormat: Text.PlainText
        font: control.font
        color: !control.enabled ? runixControlStyle.disabled
            : control.destructive ? runixControlStyle.error : runixControlStyle.text
        verticalAlignment: Text.AlignVCenter
        leftPadding: control.checkable ? 34 : 12
        rightPadding: 12
    }
    indicator: Item {
        implicitWidth: 30
        implicitHeight: 24
        Text {
            anchors.centerIn: parent
            text: control.checked ? (control.checkable ? "✓" : "•") : ""
            color: control.enabled ? runixControlStyle.focus : runixControlStyle.disabled
        }
    }
    background: Item {
        Rectangle {
            anchors.fill: parent
            color: control.highlighted ? runixControlStyle.selectedRow : "transparent"
        }
        Rectangle {
            width: 2
            anchors.left: parent.left
            anchors.top: parent.top
            anchors.bottom: parent.bottom
            color: runixControlStyle.selectionMarker
            visible: control.highlighted
        }
        Rectangle {
            anchors.fill: parent
            color: control.hovered && !control.highlighted && control.enabled ? runixControlStyle.hover : "transparent"
        }
    }
}
