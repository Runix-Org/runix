import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ApplicationWindow {
    id: window
    property var formItem: null
    property var searchInput: null
    // Hosts keep native controls as the focus/editing and debug targets.
    component Action: Loader {
        id: host
        property string text: ""
        property bool checkable: false
        property bool checked: false
        property bool autoExclusive: false
        property bool destructive: false
        signal clicked()
        sourceComponent: runixButtonComponent
        onLoaded: {
            item.text = Qt.binding(function() { return host.text })
            item.checkable = Qt.binding(function() { return host.checkable })
            item.checked = Qt.binding(function() { return host.checked })
            item.destructive = Qt.binding(function() { return host.destructive })
            if (autoExclusive) navigation.addButton(item)
            item.checkedChanged.connect(function() { host.checked = item.checked })
            item.clicked.connect(host.clicked)
        }
    }
    ButtonGroup { id: navigation }
    function registerControl(id, control, role, label, actions, activate, secret) {
        semanticDebug.registerControl(id, control instanceof Loader ? control.item : control,
            role, label, actions, activate, secret)
    }
    readonly property var appearanceMode: appearanceModeLoader.item
    readonly property var improveContrast: improveContrastLoader.item
    readonly property var lightTheme: lightThemeLoader.item
    readonly property var darkTheme: darkThemeLoader.item
    readonly property var passwordFixture: passwordFixtureLoader.item
    readonly property var geometry: geometryLoader.item
    palette.window: activeAppearance.surfaces.b0
    palette.windowText: activeAppearance.foregrounds.primary
    palette.text: activeAppearance.foregrounds.primary
    palette.buttonText: activeAppearance.foregrounds.primary
    palette.placeholderText: activeAppearance.foregrounds.placeholder
    palette.base: activeAppearance.surfaces.b1
    palette.button: activeAppearance.surfaces.b0
    palette.highlight: activeAppearance.surfaces.selection
    palette.highlightedText: activeAppearance.foregrounds["selected-text"]
    background: Rectangle {
        gradient: Gradient {
            GradientStop { position: 0; color: activeAppearance.surfaces.b0 }
            GradientStop { position: 0.7; color: activeAppearance.surfaces.b1 }
            GradientStop { position: 1; color: activeAppearance.surfaces.b1 }
        }
    }
    onVisibleChanged: {
        if (!visible && formItem) formItem.cancel()
    }
    visible: !launcherStartHidden
    flags: Qt.Window | Qt.FramelessWindowHint
    title: "runix UI lab"
    onClosing: Qt.quit()
    Component.onCompleted: {
        windowPolicy.attach(window)
        launcher.window = window
        if (typeof semanticDebug !== "undefined") {
            semanticDebug.window = window
            semanticDebug.dismiss = function() { window.requestVisibility("hide") }
            window.registerControl("quit", quit, "button", "Quit",
                ["focus", "activate", "key"], function() { quit.clicked() }, false)
            window.registerControl("password-fixture", passwordFixture, "password", "Password fixture", [], null, true)
            window.registerControl("disabled-fixture", disabledFixture, "button", "Disabled fixture",
                ["activate"], function() { disabledFixture.clicked() }, false)
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
        anchors.margins: runixControlStyle.spacing * 2
        spacing: runixControlStyle.spacing
        RowLayout {
            Label {
                Layout.fillWidth: true
                Layout.minimumWidth: 0
                elide: Text.ElideRight
                text: "UI lab"
                font.pixelSize: 20
                MouseArea { anchors.fill: parent; onPressed: windowPolicy.startMove() }
            }
            Action { text: "Window"; onClicked: geometry.open() }
            Action {
                id: quit
                text: "Quit"
                onClicked: Qt.quit()
            }
        }
        ColumnLayout {
            id: appearanceControls
            Layout.fillWidth: true
            function save(key, value) {
                const next = Object.assign({}, activeAppearance.preferences)
                next[key] = value
                activeAppearance.setPreferences(next)
            }
            RowLayout {
                Layout.fillWidth: true
                Label { text: "Appearance (development)" }
                Loader {
                    id: appearanceModeLoader
                    sourceComponent: runixComboBoxComponent
                    onLoaded: {
                        item.model = ["System", "Light", "Dark"]
                        item.currentIndex = Qt.binding(function() {
                            return ["system", "light", "dark"].indexOf(activeAppearance.preferences.mode)
                        })
                        item.activated.connect(function(index) {
                            appearanceControls.save("mode", ["system", "light", "dark"][index])
                        })
                    }
                }
                Loader {
                    id: improveContrastLoader
                    sourceComponent: runixCheckBoxComponent
                    onLoaded: {
                        item.text = "Improve contrast"
                        item.checked = Qt.binding(function() { return activeAppearance.preferences.improve_contrast })
                        item.clicked.connect(function() { appearanceControls.save("improve_contrast", item.checked) })
                    }
                }
            }
            RowLayout {
                Layout.fillWidth: true
                Label { text: "Light" }
                Loader {
                    id: lightThemeLoader
                    Layout.fillWidth: true
                    sourceComponent: runixComboBoxComponent
                    onLoaded: {
                        item.model = Qt.binding(function() { return activeAppearance.lightThemes })
                        item.textRole = "name"
                        item.currentIndex = Qt.binding(function() {
                            return item.model.findIndex(function(entry) {
                                return entry.id === activeAppearance.preferences.light_id
                            })
                        })
                        item.activated.connect(function(index) {
                            appearanceControls.save("light_id", item.model[index].id)
                        })
                    }
                }
                Label { text: "Dark" }
                Loader {
                    id: darkThemeLoader
                    Layout.fillWidth: true
                    sourceComponent: runixComboBoxComponent
                    onLoaded: {
                        item.model = Qt.binding(function() { return activeAppearance.darkThemes })
                        item.textRole = "name"
                        item.currentIndex = Qt.binding(function() {
                            return item.model.findIndex(function(entry) {
                                return entry.id === activeAppearance.preferences.dark_id
                            })
                        })
                        item.activated.connect(function(index) {
                            appearanceControls.save("dark_id", item.model[index].id)
                        })
                    }
                }
            }
            Label {
                id: appearanceStatus
                Layout.fillWidth: true
                wrapMode: Text.Wrap
                textFormat: Text.PlainText
                color: activeAppearance.foregrounds.secondary
                text: activeAppearance.name + " | System: " + activeAppearance.scheme
                    + (activeAppearance.fallbackReason ? " | " + activeAppearance.fallbackReason : "")
                    + (activeAppearance.error ? " | " + activeAppearance.error : "")
            }
            Component.onCompleted: {
                if (typeof semanticDebug === "undefined") return
                window.registerControl("appearance-mode", appearanceMode, "combobox", "Appearance mode",
                    ["focus", "select"], null, false)
                semanticDebug.controls["appearance-mode"].select = function(text) {
                    const index = ["system", "light", "dark"].indexOf(text)
                    if (index < 0) return false
                    appearanceMode.activated(index)
                    return true
                }
                window.registerControl("appearance-contrast", improveContrast, "checkbox", "Improve contrast",
                    ["focus", "activate"], function() { improveContrast.toggle(); improveContrast.clicked() }, false)
                for (const pair of [["appearance-light", lightTheme], ["appearance-dark", darkTheme]]) {
                    const control = pair[1]
                    window.registerControl(pair[0], control, "combobox", pair[0], ["focus", "select"], null, false)
                    semanticDebug.controls[pair[0]].select = function(text) {
                        const index = control.model.findIndex(function(entry) { return entry.id === text })
                        if (index < 0) return false
                        control.currentIndex = index
                        control.activated(index)
                        return true
                    }
                }
                window.registerControl("appearance-status", appearanceStatus, "status", "Active appearance", [], null, false)
                semanticDebug.controls["appearance-status"].inspect = function() {
                    const state = activeAppearance.snapshot
                    return {
                        preferences: state.preferences, active_id: state.active_id, name: state.name,
                        appearance: state.appearance, scheme: state.scheme,
                        fallback_reason: state.fallback_reason, error: state.error,
                        roles: {
                            original: state.roles.original, improve_contrast: state.roles.improve_contrast,
                            surfaces: state.roles.surfaces,
                            foregrounds: state.roles.foregrounds.map(function(role) {
                                return {name: role.name, original: role.original,
                                    effective: role.effective, diagnostic: role.diagnostic}
                            })
                        }
                    }
                }
            }
        }
        RowLayout {
            Action {
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
            Action {
                id: tableMode
                text: "Table"
                checkable: true
                checked: false
                autoExclusive: true
                onClicked: checked = true
            }
            Action {
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
            Action {
                id: galleryMode
                text: "Controls"
                checkable: true
                checked: false
                autoExclusive: true
                onClicked: {
                    checked = true
                    if (controlsGallery.item && controlsGallery.item.firstInput) {
                        launcher.query = controlsGallery.item.firstInput
                        launcher.query.forceActiveFocus(Qt.TabFocusReason)
                    }
                }
            }
            Component.onCompleted: {
                if (typeof semanticDebug === "undefined") return
                window.registerControl("search-mode", searchMode, "button", "Search mode", ["activate"], function() { searchMode.checked = true; searchMode.clicked() }, false)
                window.registerControl("table-mode", tableMode, "button", "Table mode", ["activate"], function() { tableMode.checked = true; tableMode.clicked() }, false)
                window.registerControl("form-mode", formMode, "button", "Form mode", ["activate"], function() { formMode.checked = true; formMode.clicked() }, false)
                window.registerControl("gallery-mode", galleryMode, "button", "Controls gallery", ["activate"], function() { galleryMode.checked = true; galleryMode.clicked() }, false)
            }
        }
        Loader {
            id: controlsGallery
            Layout.fillWidth: true
            Layout.fillHeight: true
            visible: galleryMode.checked
            active: true
            sourceComponent: controlsGalleryComponent
            onLoaded: {
                if (galleryMode.checked && item.firstInput) {
                    launcher.query = item.firstInput
                    launcher.query.forceActiveFocus(Qt.TabFocusReason)
                }
            }
        }
        Connections {
            target: window
            function onVisibleChanged() {
                if (!window.visible && controlsGallery.item) controlsGallery.item.clearPassword()
            }
        }
        Repeater {
            model: searchState
            delegate: ColumnLayout {
                id: screen
                Layout.fillWidth: true
                visible: !formMode.checked && !galleryMode.checked
                Layout.fillHeight: true
                readonly property var info: JSON.parse(model.snapshot)
                readonly property var query: queryLoader.item
                readonly property var iconSize: iconSizeLoader.item
                function submit() { model.command = JSON.stringify({op: "query", text: query.text}) }
                Component.onCompleted: {
                    launcher.query = query
                    window.searchInput = query
                    if (typeof semanticDebug === "undefined") return
                    window.registerControl("query", query, "textbox", "Query",
                        ["focus", "replace", "select", "key"], null, false)
                    window.registerControl("status", status, "status", "Search status", [], null, false)
                    window.registerControl("run-query", runQuery, "button", "Run query",
                        ["focus", "activate", "key"], function() { runQuery.clicked() }, false)
                    window.registerControl("results", results, "list", "Search results",
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
                    Loader {
                        id: queryLoader
                        Layout.fillWidth: true
                        sourceComponent: runixTextFieldComponent
                        onLoaded: {
                            item.search = true
                            item.placeholderText = "Search by title, workspace, category or ID"
                            item.focus = true
                            item.textChanged.connect(screen.submit)
                            item.accepted.connect(function() { results.item.activate() })
                        }
                    }
                    Connections {
                        target: query ? query.Keys : null
                        function onUpPressed(event) { results.item.move(-1); event.accepted = true }
                        function onDownPressed(event) { results.item.move(1); event.accepted = true }
                        function onPressed(event) {
                            if (event.key === Qt.Key_PageDown || event.key === Qt.Key_PageUp) {
                                results.item.move(event.key === Qt.Key_PageDown ? 5 : -5)
                                event.accepted = true
                            }
                        }
                    }
                    Action { id: runQuery; text: "Search"; onClicked: screen.submit() }
                }
                RowLayout {
                    visible: searchMode.checked
                    Action {
                        id: iconTheme
                        text: "Switch icon theme"
                        onClicked: model.command = JSON.stringify({op: "icons-theme"})
                    }
                    Action {
                        id: iconRefresh
                        text: "Refresh icons"
                        onClicked: model.command = JSON.stringify({op: "icons-refresh"})
                    }
                    Loader {
                        id: iconSizeLoader
                        sourceComponent: runixComboBoxComponent
                        onLoaded: {
                            item.model = [16, 24, 32, 48]
                            item.currentIndex = 2
                            item.activated.connect(function() {
                                if (results.item) results.item.iconSize = Number(item.currentText)
                            })
                        }
                    }
                    Component.onCompleted: {
                        if (typeof semanticDebug === "undefined") return
                        window.registerControl("icon-theme", iconTheme, "button", "Switch icon theme",
                            ["activate"], function() { iconTheme.clicked() }, false)
                        window.registerControl("icon-refresh", iconRefresh, "button", "Refresh icons",
                            ["activate"], function() { iconRefresh.clicked() }, false)
                        window.registerControl("icon-size", iconSize, "combobox", "Icon size",
                            ["focus", "key"], null, false)
                    }
                }
                Label {
                    visible: searchMode.checked
                    id: status
                    color: runixControlStyle.secondary
                    Layout.fillWidth: true
                    text: screen.info.loading ? "Searching…" : screen.info.count + " results · "
                        + screen.info.latency_ms.toFixed(2) + " ms · Selected ID: " + (screen.info.selected_id || "none")
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
                            Action { id: updateRecord; text: "Update"; enabled: !!tableScreen.tableInfo.selected_id
                                onClicked: model.command = JSON.stringify({op: "update", id: tableScreen.tableInfo.selected_id}) }
                            Action { id: deleteRecord; text: "Delete"; destructive: true; enabled: !!tableScreen.tableInfo.selected_id
                                onClicked: model.command = JSON.stringify({op: "remove", id: tableScreen.tableInfo.selected_id}) }
                            Action { id: emptyFixture; text: "Empty"; onClicked: model.command = JSON.stringify({op: "fixture", index: 0}) }
                            Action { id: singleFixture; text: "Single"; onClicked: model.command = JSON.stringify({op: "fixture", index: 1}) }
                            Action { id: resetFixture; text: "Reset"; onClicked: model.command = JSON.stringify({op: "fixture", index: 10000}) }
                        }
                        Label {
                            Layout.fillWidth: true
                            text: tableScreen.tableInfo.count + " records · Selected ID: "
                                + (tableScreen.tableInfo.selected_id || "none") + " · Activated ID: "
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
                            window.registerControl("table", tableViewLoader, "table", "Records",
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
                            window.registerControl("table-update", updateRecord, "button", "Update record",
                                ["activate"], function() { updateRecord.clicked() }, false)
                            window.registerControl("table-delete", deleteRecord, "button", "Delete record",
                                ["activate"], function() { deleteRecord.clicked() }, false)
                            window.registerControl("table-empty", emptyFixture, "button", "Empty table fixture",
                                ["activate"], function() { emptyFixture.clicked() }, false)
                            window.registerControl("table-single", singleFixture, "button", "Single table fixture",
                                ["activate"], function() { singleFixture.clicked() }, false)
                            window.registerControl("table-reset", resetFixture, "button", "Reset table fixture",
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
                    handle: Rectangle {
                        implicitWidth: 6
                        color: SplitHandle.pressed ? runixControlStyle.press
                            : SplitHandle.hovered ? runixControlStyle.hover : runixControlStyle.separator
                    }
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
                    text: "Activated ID: " + (screen.info.activated_id || "none")
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
            color: runixControlStyle.secondary
            text: "Resize ↘"
            MouseArea { anchors.fill: parent; onPressed: windowPolicy.startResize() }
        }
    }
    Loader {
        id: geometryLoader
        sourceComponent: runixPopupComponent
        onLoaded: {
            item.parent = window.contentItem
            item.anchors.centerIn = window.contentItem
            item.width = Qt.binding(function() { return Math.min(440, window.width - 32) })
            item.height = Qt.binding(function() { return Math.min(420, window.height - 32) })
            item.modalScrim = true
            geometryContent.parent = item.contentItem
        }
    }
    ScrollView {
            id: geometryContent
            visible: geometry && geometry.opened
            anchors.fill: parent
            ScrollBar.vertical: runixScrollBarComponent.createObject(null, {policy: ScrollBar.AsNeeded})
            ScrollBar.horizontal: runixScrollBarComponent.createObject(null, {policy: ScrollBar.AsNeeded})
            contentWidth: availableWidth
            contentHeight: geometryLayout.implicitHeight + 8
            ColumnLayout {
                id: geometryLayout
                x: 4
                y: 4
                width: Math.max(0, geometryContent.availableWidth - 8)
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
                    Action { text: "Compact"; onClicked: windowPolicy.expanded = false }
                    Action { text: "Expanded"; onClicked: windowPolicy.expanded = true }
                }
                Action {
                    text: "Hide for 1 second"
                    onClicked: { geometry.close(); window.requestVisibility("hide"); showAgain.start() }
                }
                Loader {
                    id: passwordFixtureLoader
                    Layout.fillWidth: true
                    visible: typeof semanticDebug !== "undefined"
                    sourceComponent: runixTextFieldComponent
                    onLoaded: {
                        item.placeholderText = "Password fixture"
                        item.text = "SEMANTIC_PASSWORD_SECRET"
                        item.echoMode = TextInput.Password
                    }
                }
                Action {
                    id: disabledFixture
                    text: "Disabled action fixture"
                    enabled: false
                    visible: typeof semanticDebug !== "undefined"
                }
                Action { text: "Close"; onClicked: geometry.close() }
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
                function onAttachedChanged() {
                    if (windowPolicy.attached && window.visible) model.request = windowPolicy.constrainedSize
                }
            }
            Connections {
                target: window
                function onVisibleChanged() {
                    if (windowPolicy.attached && window.visible) model.request = windowPolicy.constrainedSize
                }
                function onDevicePixelRatioChanged() {
                    if (windowPolicy.attached && window.visible) model.request = windowPolicy.constrainedSize
                }
            }
        }
    }
}
