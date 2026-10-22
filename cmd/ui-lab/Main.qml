import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ApplicationWindow {
    id: window
    visible: true
    width: 640
    height: 260
    title: "runix UI laboratory"

    Shortcut {
        sequence: "Escape"
        onActivated: window.close()
    }

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 24
        spacing: 12

        Label {
            text: "Go / QML bridge"
            font.pixelSize: 22
        }

        Repeater {
            model: bridge
            delegate: ColumnLayout {
                Layout.fillWidth: true
                spacing: 12

                TextField {
                    id: query
                    Layout.fillWidth: true
                    placeholderText: "Enter a query"
                    focus: true
                    onAccepted: model.command = text
                }

                Label {
                    Layout.fillWidth: true
                    text: model.status
                    textFormat: Text.PlainText
                    wrapMode: Text.Wrap
                }

                RowLayout {
                    Button {
                        text: "Run query"
                        onClicked: model.command = query.text
                    }
                    Button {
                        text: "Quit"
                        onClicked: window.close()
                    }
                }
            }
        }
        Item { Layout.fillHeight: true }
    }
}
