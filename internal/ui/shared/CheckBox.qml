import QtQuick
import QtQuick.Controls

CheckBox {
    id: control
    spacing: runixControlStyle.spacing
    background: Rectangle {
        color: runixControlStyle.wash(control)
    }
    contentItem: Text {
        text: control.text
        font: control.font
        color: runixControlStyle.foreground(control)
        verticalAlignment: Text.AlignVCenter
        leftPadding: control.indicator.width + control.spacing
    }
    indicator: Item {
        implicitWidth: 18
        implicitHeight: 18
        x: control.text ? 0 : (control.width - width) / 2
        y: (control.height - height) / 2
        Rectangle {
            anchors.fill: parent
            radius: 4
            color: control.checked && control.enabled ? runixControlStyle.selectedFill : "transparent"
            border.width: control.checked && control.enabled ? 0 : 1
            border.color: control.enabled ? runixControlStyle.unchecked : runixControlStyle.disabled
            Text {
                anchors.centerIn: parent
                text: control.checkState === Qt.PartiallyChecked ? "−" : "✓"
                visible: control.checked || control.checkState === Qt.PartiallyChecked
                color: control.enabled && control.checked ? runixControlStyle.onChecked : runixControlStyle.disabled
                font.pixelSize: 13
            }
        }
        Rectangle {
            anchors.fill: parent
            anchors.margins: -2
            radius: 6
            color: "transparent"
            border.width: runixControlStyle.focusVisible(control) ? 2 : 0
            border.color: runixControlStyle.focus
        }
    }
}
