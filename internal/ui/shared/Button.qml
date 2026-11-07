import QtQuick
import QtQuick.Controls
import QtQuick.Controls.impl

Button {
    id: control
    property bool iconOnly: false
    property bool destructive: false
    property bool busy: false
    implicitWidth: iconOnly ? 36 : Math.max(84, contentItem.implicitWidth + leftPadding + rightPadding)
    implicitHeight: 36
    padding: 9
    leftPadding: iconOnly ? 0 : 13
    rightPadding: iconOnly ? 0 : 13
    display: iconOnly ? AbstractButton.IconOnly : AbstractButton.TextBesideIcon
    icon.width: 18
    icon.height: 18
    icon.color: !enabled ? runixControlStyle.disabled
        : destructive ? runixControlStyle.errorIcon : runixControlStyle.icon
    contentItem: Item {
        implicitWidth: label.implicitWidth + (control.busy ? 22 : 0)
        implicitHeight: Math.max(label.implicitHeight, 18)
        Text {
            anchors.left: parent.left
            anchors.verticalCenter: parent.verticalCenter
            text: "◌"
            font.pixelSize: 18
            color: runixControlStyle.loader
            visible: control.busy
            RotationAnimator on rotation {
                from: 0
                to: 360
                duration: 900
                loops: Animation.Infinite
                running: control.busy && control.visible
            }
        }
        IconLabel {
            id: label
            anchors.fill: parent
            anchors.leftMargin: control.busy ? 22 : 0
            icon: control.icon
            text: control.text
            font: control.font
            spacing: 6
            mirrored: control.mirrored
            display: control.iconOnly && !control.icon.name && !control.icon.source.toString()
                ? AbstractButton.TextOnly : control.display
            color: !control.enabled ? runixControlStyle.disabled
                : control.iconOnly ? control.icon.color
                    : control.destructive ? runixControlStyle.error : runixControlStyle.text
        }
    }
    background: Item {
        Rectangle {
            anchors.fill: parent
            anchors.margins: -2
            radius: runixControlStyle.radius + 2
            color: "transparent"
            border.width: runixControlStyle.focusVisible(control) ? 2 : 0
            border.color: runixControlStyle.focus
        }
        Rectangle {
            anchors.fill: parent
            radius: runixControlStyle.radius
            color: control.iconOnly ? "transparent" : runixControlStyle.button
            border.width: control.iconOnly ? 0 : 1
            border.color: runixControlStyle.separator
        }
        Rectangle {
            anchors.fill: parent
            radius: runixControlStyle.radius
            color: runixControlStyle.wash(control)
        }
    }
}
