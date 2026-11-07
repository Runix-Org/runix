import QtQuick
import QtQuick.Controls

Switch {
    id: control
    spacing: runixControlStyle.spacing
    contentItem: Text {
        text: control.text
        font: control.font
        color: runixControlStyle.foreground(control)
        verticalAlignment: Text.AlignVCenter
        leftPadding: control.indicator.width + control.spacing
    }
    indicator: Item {
        implicitWidth: 38
        implicitHeight: 22
        x: control.text ? 0 : (control.width - width) / 2
        y: (control.height - height) / 2
        Rectangle {
            anchors.fill: parent
            radius: height / 2
            color: !control.enabled ? runixControlStyle.disabledTrack
                : control.checked ? runixControlStyle.selectedFill : runixControlStyle.track
        }
        Rectangle {
            anchors.fill: parent
            radius: height / 2
            color: runixControlStyle.wash(control)
        }
        Rectangle {
            width: 16
            height: 16
            y: 3
            x: control.checked ? parent.width - width - 3 : 3
            radius: width / 2
            color: !control.enabled ? runixControlStyle.disabled
                : control.checked ? runixControlStyle.onChecked : runixControlStyle.switchThumb
        }
        Rectangle {
            anchors.fill: parent
            anchors.margins: -2
            radius: height / 2
            color: "transparent"
            border.width: runixControlStyle.focusVisible(control) ? 2 : 0
            border.color: runixControlStyle.focus
        }
    }
}
