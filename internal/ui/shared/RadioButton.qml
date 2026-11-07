import QtQuick
import QtQuick.Controls

RadioButton {
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
            radius: width / 2
            color: control.checked && control.enabled ? runixControlStyle.selectedFill : "transparent"
            border.width: control.checked && control.enabled ? 0 : 1
            border.color: control.enabled ? runixControlStyle.unchecked : runixControlStyle.disabled
            Rectangle {
                anchors.centerIn: parent
                width: 6
                height: 6
                radius: width / 2
                color: control.enabled ? runixControlStyle.onChecked : runixControlStyle.disabled
                visible: control.checked
            }
        }
        Rectangle {
            anchors.fill: parent
            anchors.margins: -2
            radius: width / 2
            color: "transparent"
            border.width: runixControlStyle.focusVisible(control) ? 2 : 0
            border.color: runixControlStyle.focus
        }
    }
}
