import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ApplicationWindow {
    id: window
    visible: !launcherStartHidden
    flags: Qt.Window | Qt.FramelessWindowHint
    Component.onCompleted: {
        windowPolicy.attach(window)
        launcher.window = window
        if (typeof semanticDebug !== "undefined") {
            semanticDebug.window = window
            semanticDebug.dismiss = function() { window.requestVisibility("hide") }
        }
    }
    title: "runix UI laboratory"
    onClosing: Qt.quit()

    function requestVisibility(command) {
        visibilityCommand.item.request(command)
    }

    Repeater {
        id: visibilityCommand
        model: launcherCommands
        delegate: Item {
            function request(command) { model.command = command }
        }
        property var item: count > 0 ? itemAt(0) : null
    }

    Shortcut {
        sequence: "Escape"
        onActivated: window.requestVisibility("hide")
    }

    Timer {
        id: showAgain
        interval: 1000
        onTriggered: window.requestVisibility("show")
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
                        window.requestVisibility("hide")
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
                    Component.onCompleted: {
                        launcher.query = query
                        if (typeof semanticDebug === "undefined")
                            return
                        semanticDebug.registerControl("query", query, "textbox", "Query",
                            ["focus", "replace", "select", "key"], null, false)
                        semanticDebug.registerControl("status", status, "status", "Query status", [], null, false)
                        semanticDebug.registerControl("run-query", runQuery, "button", "Run query",
                            ["focus", "activate", "key"], function() { runQuery.clicked() }, false)
                        semanticDebug.registerControl("quit", quit, "button", "Quit",
                            ["focus", "activate", "key"], function() { quit.clicked() }, false)
                        semanticDebug.registerControl("password-fixture", passwordFixture, "password", "Password fixture",
                            [], null, true)
                        semanticDebug.registerControl("disabled-fixture", disabledFixture, "button", "Disabled fixture",
                            ["activate"], function() { disabledFixture.clicked() }, false)
                    }

                    TextField {
                        id: query
                        objectName: "query"
                        Layout.fillWidth: true
                        placeholderText: "Enter a query"
                        focus: true
                        onActiveFocusChanged: {
                            if (activeFocus)
                                viewport.contentItem.contentY = Math.max(0, Math.min(
                                    viewport.contentHeight - viewport.availableHeight, query.mapToItem(content, 0, 0).y))
                        }
                        onAccepted: model.command = text
                    }

                    Label {
                        id: status
                        objectName: "status"
                        Layout.fillWidth: true
                        text: model.status
                        textFormat: Text.PlainText
                        wrapMode: Text.Wrap
                    }

                    TextField {
                        id: passwordFixture
                        objectName: "password-fixture"
                        Layout.fillWidth: true
                        visible: typeof semanticDebug !== "undefined"
                        placeholderText: "Password fixture"
                        text: "SEMANTIC_PASSWORD_SECRET"
                        echoMode: TextInput.Password
                    }

                    Button {
                        id: disabledFixture
                        objectName: "disabled-fixture"
                        text: "Disabled action fixture"
                        enabled: false
                        visible: typeof semanticDebug !== "undefined"
                    }

                    RowLayout {
                        Button {
                            id: runQuery
                            objectName: "run-query"
                            text: "Run query"
                            onClicked: model.command = query.text
                        }
                        Button {
                            id: quit
                            objectName: "quit"
                            text: "Quit"
                            onClicked: Qt.quit()
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
