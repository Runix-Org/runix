import QtQuick
import QtQuick.Controls

ScrollBar {
    id: control
    x: orientation === Qt.Vertical && parent ? parent.width - width : 0
    y: orientation === Qt.Horizontal && parent ? parent.height - height : 0
    width: orientation === Qt.Horizontal && parent ? parent.width : implicitWidth
    height: orientation === Qt.Vertical && parent ? parent.height : implicitHeight
    implicitWidth: orientation === Qt.Vertical ? 10 : 24
    background: Rectangle { color: "transparent" }
    implicitHeight: orientation === Qt.Vertical ? 24 : 10
    padding: 2
    contentItem: Rectangle {
        implicitWidth: control.orientation === Qt.Vertical ? 6 : 24
        implicitHeight: control.orientation === Qt.Vertical ? 24 : 6
        radius: width / 2
        color: !control.enabled ? runixControlStyle.disabledTrack
            : control.pressed ? runixControlStyle.scrollPressed
                : control.hovered ? runixControlStyle.scrollHover : runixControlStyle.scrollThumb
    }
}
