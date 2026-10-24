import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ApplicationWindow {
    id: window
    visible: !launcherStartHidden
    flags: Qt.Window | Qt.FramelessWindowHint
    title: "runix UI laboratory"
    onClosing: Qt.quit()
    Component.onCompleted: {
        windowPolicy.attach(window)
        launcher.window = window
        if (typeof semanticDebug !== "undefined") {
            semanticDebug.window = window
            semanticDebug.dismiss = function() { window.requestVisibility("hide") }
        }
    }
    function requestVisibility(command) { visibilityCommand.itemAt(0).request(command) }
    Repeater {
        id: visibilityCommand
        model: launcherCommands
        delegate: Item { function request(command) { model.command = command } }
    }
    Shortcut { sequence: "Escape"; onActivated: window.requestVisibility("hide") }
    Timer { id: showAgain; interval: 1000; onTriggered: window.requestVisibility("show") }

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 16
        spacing: 8
        RowLayout {
            Label {
                Layout.fillWidth: true
                text: "Search · 50,000 records"
                font.pixelSize: 20
                MouseArea { anchors.fill: parent; onPressed: windowPolicy.startMove() }
            }
            Button { text: "Window"; onClicked: geometry.open() }
            Button {
                id: quit
                text: "Quit"
                onClicked: Qt.quit()
            }
        }
        Repeater {
            model: searchState
            delegate: ColumnLayout {
                id: screen
                Layout.fillWidth: true
                Layout.fillHeight: true
                readonly property var info: JSON.parse(model.snapshot)
                function submit() { model.command = JSON.stringify({op: "query", text: query.text}) }
                Component.onCompleted: {
                    launcher.query = query
                    if (typeof semanticDebug === "undefined") return
                    semanticDebug.registerControl("query", query, "textbox", "Query",
                        ["focus", "replace", "select", "key"], null, false)
                    semanticDebug.registerControl("status", status, "status", "Search status", [], null, false)
                    semanticDebug.registerControl("run-query", runQuery, "button", "Run query",
                        ["focus", "activate", "key"], function() { runQuery.clicked() }, false)
                    semanticDebug.registerControl("quit", quit, "button", "Quit",
                        ["focus", "activate", "key"], function() { quit.clicked() }, false)
                    semanticDebug.registerControl("password-fixture", passwordFixture, "password", "Password fixture",
                        [], null, true)
                    semanticDebug.registerControl("disabled-fixture", disabledFixture, "button", "Disabled fixture",
                        ["activate"], function() { disabledFixture.clicked() }, false)
                    semanticDebug.registerControl("results", results, "list", "Search results",
                        ["focus", "select", "scroll", "activate"], function() { results.item.activate() }, false)
                    semanticDebug.controls.results.inspect = function() { return results.item.inspect() }
                    semanticDebug.controls.results.select = function(text) {
                        if (!/^[0-9]+$/.test(text)) return false
                        return results.item.select(Number(text))
                    }
                    semanticDebug.controls.results.scroll = function(text) {
                        if (!/^[0-9]+$/.test(text)) return false
                        const index = Number(text)
                        if (index >= results.item.view.count) return false
                        results.item.view.positionViewAtIndex(index, ListView.Beginning)
                        return true
                    }
                }
                RowLayout {
                    TextField {
                        id: query
                        Layout.fillWidth: true
                        placeholderText: "Search titles, workspaces, categories or IDs"
                        focus: true
                        onTextChanged: screen.submit()
                        onAccepted: results.item.activate()
                        Keys.onUpPressed: results.item.move(-1)
                        Keys.onDownPressed: results.item.move(1)
                        Keys.onPressed: function(event) {
                            if (event.key === Qt.Key_PageDown || event.key === Qt.Key_PageUp) {
                                results.item.move(event.key === Qt.Key_PageDown ? 5 : -5)
                                event.accepted = true
                            }
                        }
                    }
                    Button { id: runQuery; text: "Search"; onClicked: screen.submit() }
                }
                Label {
                    id: status
                    Layout.fillWidth: true
                    text: screen.info.loading ? "Searching…" : screen.info.count + " results · "
                        + screen.info.latency_ms.toFixed(2) + " ms · selected: " + (screen.info.selected_id || "none")
                    textFormat: Text.PlainText
                    elide: Text.ElideRight
                }
                Loader {
                    id: results
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    Layout.minimumHeight: 54
                    sourceComponent: resultListComponent
                    onLoaded: item.state = model
                }
                Label {
                    Layout.fillWidth: true
                    text: "Activated: " + (screen.info.activated_id || "none")
                    textFormat: Text.PlainText
                }
            }
        }
        Label {
            Layout.alignment: Qt.AlignRight
            text: "Resize ↘"
            MouseArea { anchors.fill: parent; onPressed: windowPolicy.startResize() }
        }
    }
    Popup {
        id: geometry
        anchors.centerIn: parent
        width: Math.min(440, window.width - 32)
        height: Math.min(420, window.height - 32)
        modal: true
        ScrollView {
            anchors.fill: parent
            contentWidth: availableWidth
            ColumnLayout {
                width: parent.width
                Label {
                    Layout.fillWidth: true
                    text: "Requested: " + windowPolicy.requestedSize.width + " × " + windowPolicy.requestedSize.height
                        + "\nConstrained: " + windowPolicy.constrainedSize.width + " × " + windowPolicy.constrainedSize.height
                        + "\nObserved: " + window.width + " × " + window.height
                        + "\nQt screen DPR: " + windowPolicy.scale + " | Output: " + window.Screen.name
                        + "\nLimits: " + window.minimumWidth + " × " + window.minimumHeight
                        + " — " + window.maximumWidth + " × " + window.maximumHeight
                        + "\nAvailable: " + windowPolicy.availableWidth + " × " + windowPolicy.availableHeight
                        + "\nMove: " + windowPolicy.moveResult + "\nResize: " + windowPolicy.resizeResult
                    wrapMode: Text.Wrap
                }
                Label {
                    id: windowCommandStatus
                    Layout.fillWidth: true
                    wrapMode: Text.Wrap
                    textFormat: Text.PlainText
                }
                RowLayout {
                    Button { text: "Compact"; onClicked: windowPolicy.expanded = false }
                    Button { text: "Expanded"; onClicked: windowPolicy.expanded = true }
                }
                Button {
                    text: "Hide for 1 second"
                    onClicked: { geometry.close(); window.requestVisibility("hide"); showAgain.start() }
                }
                TextField {
                    id: passwordFixture
                    Layout.fillWidth: true
                    visible: typeof semanticDebug !== "undefined"
                    placeholderText: "Password fixture"
                    text: "SEMANTIC_PASSWORD_SECRET"
                    echoMode: TextInput.Password
                }
                Button {
                    id: disabledFixture
                    text: "Disabled action fixture"
                    enabled: false
                    visible: typeof semanticDebug !== "undefined"
                }
                Button { text: "Close"; onClicked: geometry.close() }
            }
        }
    }
    Repeater {
        model: windowCommands
        delegate: Label {
            visible: false
            text: model.status
            onTextChanged: function() { windowCommandStatus.text = model.status }
            Connections {
                target: windowPolicy
                function onConstrainedSizeChanged() {
                    if (windowPolicy.attached && window.visible) model.request = windowPolicy.constrainedSize
                }
            }
        }
    }
}
