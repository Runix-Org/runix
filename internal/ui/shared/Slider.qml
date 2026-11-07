import QtQuick
import QtQuick.Controls

Slider {
    id: control
    implicitWidth: orientation === Qt.Horizontal ? 180 : 28
    implicitHeight: orientation === Qt.Horizontal ? 28 : 180
    background: Item {
        x: control.leftPadding + (control.availableWidth - width) / 2
        y: control.orientation === Qt.Horizontal ? control.topPadding + (control.availableHeight - height) * 0.5 : control.topPadding
        width: control.orientation === Qt.Horizontal ? control.availableWidth : 4
        height: control.orientation === Qt.Horizontal ? 4 : control.availableHeight
        Rectangle {
            anchors.fill: parent
            radius: width < height ? width / 2 : height / 2
            color: control.enabled ? runixControlStyle.track : runixControlStyle.disabledTrack
        }
        Rectangle {
            x: control.orientation === Qt.Horizontal && control.mirrored ? parent.width - width : 0
            y: control.orientation === Qt.Horizontal ? 0 : control.visualPosition * parent.height
            width: control.orientation === Qt.Horizontal ? control.position * parent.width : parent.width
            height: control.orientation === Qt.Horizontal ? parent.height : control.position * parent.height
            radius: width < height ? width / 2 : height / 2
            color: control.enabled ? runixControlStyle.sliderFill : runixControlStyle.disabled
        }
    }
    handle: Item {
        x: control.orientation === Qt.Horizontal
            ? control.leftPadding + control.visualPosition * (control.availableWidth - width)
            : control.leftPadding + (control.availableWidth - width) / 2
        y: control.orientation === Qt.Horizontal
            ? control.topPadding + (control.availableHeight - height) / 2
            : control.topPadding + control.visualPosition * (control.availableHeight - height)
        implicitWidth: 18
        implicitHeight: 18
        Rectangle {
            anchors.fill: parent
            radius: width / 2
            color: control.enabled ? runixControlStyle.sliderThumb : runixControlStyle.disabled
            border.width: 1
            border.color: activeAppearance.surfaces.border
        }
        Rectangle {
            anchors.fill: parent
            radius: width / 2
            color: runixControlStyle.wash(control)
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
