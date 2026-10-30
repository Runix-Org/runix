import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ApplicationWindow {
    id: window
    property var formItem: null
    property var searchInput: null
    readonly property var labTheme: themeLoader.item
    Loader { id: themeLoader; sourceComponent: labThemeComponent }
    palette.window: labTheme ? labTheme.surface : "#202832"
    palette.windowText: labTheme ? labTheme.foreground : "#e8edf2"
    palette.text: palette.windowText
    palette.buttonText: palette.windowText
    palette.base: labTheme ? labTheme.field : "#151c23"
    palette.button: palette.window
    palette.highlight: labTheme ? labTheme.accent : "#79b8ff"
    onVisibleChanged: {
        if (!visible && formItem) formItem.cancel()
    }
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
    function requestVisibility(command) {
        if (command === "hide" && formMode.checked && formItem) formItem.cancel()
        visibilityCommand.itemAt(0).request(command)
    }
    Repeater {
        id: visibilityCommand
        model: launcherCommands
        delegate: Item { function request(command) { model.command = command } }
    }
    Shortcut { sequence: "Escape"; onActivated: window.requestVisibility("hide") }
    Timer { id: showAgain; interval: 1000; onTriggered: window.requestVisibility("show") }

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: window.labTheme ? window.labTheme.spacing * 2 : 16
        spacing: window.labTheme ? window.labTheme.spacing : 8
        RowLayout {
            Label {
                Layout.fillWidth: true
                Layout.minimumWidth: 0
                elide: Text.ElideRight
                text: "UI laboratory"
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
        RowLayout {
            Button {
                id: searchMode
                text: "Search"
                checkable: true
                checked: true
                autoExclusive: true
                onClicked: {
                    checked = true
                    launcher.query = window.searchInput
                    if (launcher.query) launcher.query.forceActiveFocus(Qt.TabFocusReason)
                }
            }
            Button {
                id: tableMode
                text: "Table"
                checkable: true
                checked: false
                autoExclusive: true
                onClicked: checked = true
            }
            Button {
                id: formMode
                text: "Form"
                checkable: true
                autoExclusive: true
                onClicked: checked = true
                onCheckedChanged: {
                    if (!window.formItem) return
                    if (checked) {
                        launcher.query = window.formItem.initialFocus
                        launcher.query.forceActiveFocus(Qt.TabFocusReason)
                    } else {
                        window.formItem.cancel()
                    }
                }
            }
            Component.onCompleted: {
                if (typeof semanticDebug === "undefined") return
                semanticDebug.registerControl("search-mode", searchMode, "button", "Search mode",
                    ["activate"], function() { searchMode.clicked() }, false)
                semanticDebug.registerControl("table-mode", tableMode, "button", "Table mode",
                    ["activate"], function() { tableMode.clicked() }, false)
                semanticDebug.registerControl("form-mode", formMode, "button", "Form mode",
                    ["activate"], function() { formMode.clicked() }, false)
            }
        }
        Repeater {
            model: searchState
            delegate: ColumnLayout {
                id: screen
                Layout.fillWidth: true
                visible: !formMode.checked
                Layout.fillHeight: true
                readonly property var info: JSON.parse(model.snapshot)
                function submit() { model.command = JSON.stringify({op: "query", text: query.text}) }
                Component.onCompleted: {
                    launcher.query = query
                    window.searchInput = query
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
                    visible: searchMode.checked
                    TextField {
                        id: query
                        Layout.fillWidth: true
                        placeholderText: "Search titles, workspaces, categories or IDs"
                        focus: true
                        background: Rectangle {
                            color: window.palette.base
                            radius: window.labTheme ? window.labTheme.radius : 5
                            border.color: query.activeFocus ? window.palette.highlight : window.palette.mid
                            border.width: query.activeFocus ? 2 : 1
                        }
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
                RowLayout {
                    visible: searchMode.checked
                    Button {
                        id: iconTheme
                        text: "Switch icon theme"
                        onClicked: model.command = JSON.stringify({op: "icons-theme"})
                    }
                    Button {
                        id: iconRefresh
                        text: "Refresh icons"
                        onClicked: model.command = JSON.stringify({op: "icons-refresh"})
                    }
                    ComboBox {
                        id: iconSize
                        model: [16, 24, 32, 48]
                        currentIndex: 2
                        onActivated: if (results.item) results.item.iconSize = Number(currentText)
                    }
                    Component.onCompleted: {
                        if (typeof semanticDebug === "undefined") return
                        semanticDebug.registerControl("icon-theme", iconTheme, "button", "Switch icon theme",
                            ["activate"], function() { iconTheme.clicked() }, false)
                        semanticDebug.registerControl("icon-refresh", iconRefresh, "button", "Refresh icons",
                            ["activate"], function() { iconRefresh.clicked() }, false)
                        semanticDebug.registerControl("icon-size", iconSize, "combobox", "Icon size",
                            ["focus", "key"], null, false)
                    }
                }
                Label {
                    visible: searchMode.checked
                    id: status
                    Layout.fillWidth: true
                    text: screen.info.loading ? "Searching…" : screen.info.count + " results · "
                        + screen.info.latency_ms.toFixed(2) + " ms · selected: " + (screen.info.selected_id || "none")
                    textFormat: Text.PlainText
                    elide: Text.ElideRight
                }
                Repeater {
                    model: tableState
                    delegate: ColumnLayout {
                        id: tableScreen
                        visible: tableMode.checked
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        readonly property var tableInfo: JSON.parse(model.snapshot)
                        RowLayout {
                            Button { id: updateRecord; text: "Update"; enabled: !!tableScreen.tableInfo.selected_id
                                onClicked: model.command = JSON.stringify({op: "update", id: tableScreen.tableInfo.selected_id}) }
                            Button { id: deleteRecord; text: "Delete"; enabled: !!tableScreen.tableInfo.selected_id
                                onClicked: model.command = JSON.stringify({op: "remove", id: tableScreen.tableInfo.selected_id}) }
                            Button { id: emptyFixture; text: "Empty"; onClicked: model.command = JSON.stringify({op: "fixture", index: 0}) }
                            Button { id: singleFixture; text: "Single"; onClicked: model.command = JSON.stringify({op: "fixture", index: 1}) }
                            Button { id: resetFixture; text: "Reset"; onClicked: model.command = JSON.stringify({op: "fixture", index: 10000}) }
                        }
                        Label {
                            Layout.fillWidth: true
                            text: tableScreen.tableInfo.count + " records · selected: "
                                + (tableScreen.tableInfo.selected_id || "none") + " · activated: "
                                + (tableScreen.tableInfo.activated_id || "none")
                            elide: Text.ElideRight
                        }
                        Loader {
                            id: tableViewLoader
                            Layout.fillWidth: true
                            Layout.fillHeight: true
                            sourceComponent: tableViewComponent
                            onLoaded: item.state = model
                        }
                        Connections {
                            target: tableMode
                            function onCheckedChanged() {
                                if (tableMode.checked && tableViewLoader.item)
                                    launcher.query = tableViewLoader.item.view
                                else if (!formMode.checked)
                                    launcher.query = query
                            }
                        }
                        Component.onCompleted: {
                            if (typeof semanticDebug === "undefined") return
                            semanticDebug.registerControl("table", tableViewLoader, "table", "Records",
                                ["focus", "select", "scroll", "activate"],
                                function() { tableViewLoader.item.activate() }, false)
                            semanticDebug.controls.table.inspect = function() { return tableViewLoader.item.inspect() }
                            semanticDebug.controls.table.select = function(text) {
                                if (!/^[0-9]+$/.test(text)) return false
                                return tableViewLoader.item.select(Number(text))
                            }
                            semanticDebug.controls.table.scroll = function(text) {
                                if (!/^[0-9]+$/.test(text)) return false
                                const index = Number(text)
                                if (index >= tableViewLoader.item.view.rows) return false
                                tableViewLoader.item.view.positionViewAtRow(index, TableView.Beginning)
                                return true
                            }
                            semanticDebug.registerControl("table-update", updateRecord, "button", "Update record",
                                ["activate"], function() { updateRecord.clicked() }, false)
                            semanticDebug.registerControl("table-delete", deleteRecord, "button", "Delete record",
                                ["activate"], function() { deleteRecord.clicked() }, false)
                            semanticDebug.registerControl("table-empty", emptyFixture, "button", "Empty table fixture",
                                ["activate"], function() { emptyFixture.clicked() }, false)
                            semanticDebug.registerControl("table-single", singleFixture, "button", "Single table fixture",
                                ["activate"], function() { singleFixture.clicked() }, false)
                            semanticDebug.registerControl("table-reset", resetFixture, "button", "Reset table fixture",
                                ["activate"], function() { resetFixture.clicked() }, false)
                            semanticDebug.controls.table.activate = function() { tableViewLoader.item.activate() }
                        }
                    }
                }
                SplitView {
                    visible: searchMode.checked
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    Layout.minimumHeight: 54
                    orientation: Qt.Horizontal
                    Loader {
                        id: results
                        SplitView.preferredWidth: 380
                        SplitView.minimumWidth: 180
                        SplitView.fillHeight: true
                        sourceComponent: resultListComponent
                        onLoaded: item.state = model
                    }
                    Repeater {
                        model: detailState
                        delegate: Loader {
                            SplitView.fillWidth: true
                            SplitView.minimumWidth: 180
                            SplitView.fillHeight: true
                            sourceComponent: detailPanelComponent
                            onLoaded: {
                                item.state = model
                                item.semanticDebug = typeof semanticDebug !== "undefined" ? semanticDebug : null
                                item.contentID = Qt.binding(function() { return screen.info.selected_id })
                            }
                        }
                    }
                }
                Label {
                    visible: searchMode.checked
                    Layout.fillWidth: true
                    text: "Activated: " + (screen.info.activated_id || "none")
                    textFormat: Text.PlainText
                }
            }
        }
        Repeater {
            model: formState
            delegate: Loader {
                id: formLoader
                visible: formMode.checked
                Layout.fillWidth: true
                Layout.fillHeight: true
                sourceComponent: formViewComponent
                onLoaded: {
                    item.state = model
                    window.formItem = item
                }
                Connections {
                    target: formLoader.item
                    function onDismissed() { window.requestVisibility("hide") }
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
