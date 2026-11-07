import QtQuick
import QtQuick.Controls

ToolTip {
    id: control
    contentItem: Text {
        text: control.text
        font: control.font
        textFormat: Text.PlainText
        color: runixControlStyle.text
        wrapMode: Text.Wrap
    }
    background: Rectangle {
        color: runixControlStyle.popup
        border.color: runixControlStyle.popupBorder
        border.width: 1
        radius: runixControlStyle.radius
    }
}
