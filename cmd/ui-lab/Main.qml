import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ApplicationWindow {
    id: window
    visible: true
    flags: Qt.Window | Qt.FramelessWindowHint
    Component.onCompleted: windowPolicy.attach(window)
    title: "runix UI laboratory"

    Shortcut {
        sequence: "Escape"
        onActivated: window.close()
    }

    Timer {
        id: showAgain
        interval: 1000
        onTriggered: window.show()
    }

    ScrollView {
        id: viewport
        anchors.fill: parent
        anchors.margins: 24
        clip: true
        contentWidth: availableWidth
        contentHeight: content.implicitHeight

        ColumnLayout {
            id: content
            width: viewport.availableWidth
            spacing: 12

            Label {
                Layout.fillWidth: true
                text: "Window policy — drag here"
                font.pixelSize: 22
                MouseArea {
                    anchors.fill: parent
                    onPressed: windowPolicy.startMove()
                }
            }

            Label {
                Layout.fillWidth: true
                text: "Requested: " + windowPolicy.requestedSize.width + " × " + windowPolicy.requestedSize.height
                    + " | Constrained: " + windowPolicy.constrainedSize.width + " × " + windowPolicy.constrainedSize.height
                    + "\nObserved (logical): " + window.width + " × " + window.height
                    + " | Qt screen DPR: " + windowPolicy.scale + " | Output: " + window.Screen.name
                    + "\nLimits: " + window.minimumWidth + " × " + window.minimumHeight
                    + " — " + window.maximumWidth + " × " + window.maximumHeight
                    + "\nAvailable (logical): " + windowPolicy.availableWidth + " × " + windowPolicy.availableHeight
                    + "\nMove: " + windowPolicy.moveResult + "\nResize: " + windowPolicy.resizeResult
                wrapMode: Text.Wrap
            }

            RowLayout {
                Button {
                    text: "Compact"
                    onClicked: windowPolicy.expanded = false
                }
                Button {
                    text: "Expanded"
                    onClicked: windowPolicy.expanded = true
                }
                Button {
                    text: "Hide for 1 second"
                    onClicked: {
                        window.hide()
                        showAgain.start()
                    }
                }
            }

            Repeater {
                model: windowCommands
                delegate: Label {
                    Layout.fillWidth: true
                    text: model.status
                    textFormat: Text.PlainText
                    wrapMode: Text.Wrap
                    Connections {
                        target: windowPolicy
                        function onConstrainedSizeChanged() {
                            if (windowPolicy.attached && window.visible)
                                model.request = windowPolicy.constrainedSize
                        }
                    }
                }
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
            Label {
                Layout.alignment: Qt.AlignRight
                text: "Resize ↘"
                MouseArea {
                    anchors.fill: parent
                    onPressed: windowPolicy.startResize()
                }
            }
        }
    }
}
