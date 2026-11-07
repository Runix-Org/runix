import QtQuick
import QtQuick.Controls
import QtQuick.Window

ComboBox {
    id: control
    implicitWidth: Math.max(120, contentItem.implicitWidth + 52)
    implicitHeight: 36
    property Item invokingFocusItem: null
    contentItem: Text {
        leftPadding: 12
        rightPadding: 28
        text: control.displayText
        textFormat: Text.PlainText
        font: control.font
        color: !control.enabled ? runixControlStyle.disabled : runixControlStyle.text
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideRight
    }
    indicator: Text {
        x: control.width - width - 12
        y: (control.height - height) / 2
        text: "▾"
        color: control.enabled ? runixControlStyle.icon : runixControlStyle.disabled
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
            color: runixControlStyle.surface
            border.color: !control.enabled ? runixControlStyle.disabledBorder
                : control.hovered ? runixControlStyle.hoverBorder : runixControlStyle.fieldBorder
        }
    }
    delegate: ItemDelegate {
        id: row
        required property int index
        required property var model
        required property var modelData
        width: control.popup.width
        height: 36
        text: control.textRole ? model[control.textRole] : modelData
        highlighted: control.highlightedIndex === index
        contentItem: Text {
            text: row.text
            textFormat: Text.PlainText
            color: !row.enabled ? runixControlStyle.disabled : runixControlStyle.text
            verticalAlignment: Text.AlignVCenter
            leftPadding: 24
        }
        background: Item {
            Rectangle {
                anchors.fill: parent
                color: row.highlighted ? runixControlStyle.selectedRow : "transparent"
            }
            Rectangle {
                width: 2
                anchors.left: parent.left
                anchors.top: parent.top
                anchors.bottom: parent.bottom
                color: runixControlStyle.selectionMarker
                visible: row.highlighted
            }
            Rectangle {
                anchors.fill: parent
                color: row.hovered && !row.highlighted && row.enabled ? runixControlStyle.hover : "transparent"
            }
            Text {
                anchors.left: parent.left
                anchors.leftMargin: 8
                anchors.verticalCenter: parent.verticalCenter
                text: row.index === control.currentIndex ? "✓" : ""
                color: row.enabled ? runixControlStyle.focus : runixControlStyle.disabled
            }
        }
    }
    popup: Popup {
        y: control.height
        width: control.width
        implicitHeight: contentItem.implicitHeight
        padding: 4
        onAboutToShow: control.invokingFocusItem = Window.activeFocusItem
        onClosed: {
            if (control.invokingFocusItem) control.invokingFocusItem.forceActiveFocus()
            control.invokingFocusItem = null
        }
        contentItem: ListView {
            clip: true
            implicitHeight: contentHeight
            model: control.popup.visible ? control.delegateModel : null
            currentIndex: control.highlightedIndex
            ScrollBar.vertical: runixScrollBarComponent.createObject(null)
        }
        background: Rectangle {
            color: runixControlStyle.popup
            border.color: runixControlStyle.popupBorder
            border.width: 1
            radius: runixControlStyle.radius
        }
    }
}
