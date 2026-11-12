import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ApplicationWindow {
    id: window
    // Hosts only: the engine-owned controls retain native focus and editing.
    component Action: Loader {
        id: action
        property string text: ""
        property bool busy: false
        property bool primary: false
        property bool destructive: false
        signal clicked()
        sourceComponent: runixButtonComponent
        onLoaded: {
            item.text = Qt.binding(function() { return action.text })
            item.busy = Qt.binding(function() { return action.busy })
            item.font.bold = Qt.binding(function() { return action.primary })
            item.destructive = Qt.binding(function() { return action.destructive })
            item.clicked.connect(action.clicked)
        }
    }
    component Notice: RowLayout {
        id: notice
        property string text: ""
        property string hue: "blue"
        Layout.fillWidth: true
        visible: text.length > 0
        spacing: runixControlStyle.spacing
        Label {
            text: ({red: "!", yellow: "⚠", green: "✓", blue: "i"})[notice.hue] || "i"
            color: runixControlStyle.hueIcon(notice.hue)
            Layout.alignment: Qt.AlignTop
        }
        Label {
            Layout.fillWidth: true
            text: notice.text
            textFormat: Text.PlainText
            color: runixControlStyle.text
            wrapMode: Text.Wrap
        }
    }
    visible: false
    title: "runix"
    flags: Qt.Window | Qt.FramelessWindowHint
    onClosing: Qt.quit()
    property var launchInfo: ({busy: false})
    property bool integrationOpen: false
    property bool integrationStartupResolved: false
    property bool shortcutStarted: false
    property bool shortcutStartupResolved: false
    property int searchResetSignal: 0
    property bool settingsOpen: false
    property bool onboarding: false
    property var shortcutInfo: ({busy: true, onboarding: false, available: false})
    function shortcutRequest(action) {
        const panel = shortcutPanel.itemAt(0)
        if (panel) panel.request(action)
    }
    function startShortcutSetup() {
        if (!integrationStartupResolved || integrationOpen || shortcutStarted) return
        shortcutStarted = true
        shortcutRequest("startup")
    }
    function openIntegration() {
        shortcutRequest("cancel")
        integrationOpen = true
        integrationPanel.itemAt(0).request("refresh")
    }
    function openSettingsFromCommand() {
        if (!window.visible || window.settingsOpen || window.integrationOpen || window.onboarding) return false
        window.settingsOpen = true
        integrationPanel.itemAt(0).request("refresh")
        window.shortcutRequest("refresh")
        return true
    }
    function backFromSettings() {
        if (!window.onboarding) window.searchResetSignal++
        window.settingsOpen = false
    }
    property string settingsTab: "appearance"
    onSettingsOpenChanged: { if (!settingsOpen) shortcutRequest("cancel") }
    onSettingsTabChanged: { if (settingsTab !== "shortcut") shortcutRequest("cancel") }
    onVisibleChanged: { if (!visible) shortcutRequest("cancel") }
    Repeater {
        id: launchRepeater
        model: launchState
        delegate: Item {
            readonly property var info: JSON.parse(model.snapshot)
            onInfoChanged: window.launchInfo = info
            Connections {
                target: window
                function onVisibleChanged() { model.command = window.visible }
            }
        }
    }
    readonly property color themeForeground: activeAppearance.foregrounds.primary
    readonly property color themeSurface: activeAppearance.surfaces.b0
    palette.window: themeSurface
    palette.windowText: themeForeground
    palette.text: themeForeground
    palette.buttonText: themeForeground
    palette.placeholderText: activeAppearance.foregrounds.placeholder
    palette.base: activeAppearance.surfaces.b1
    palette.button: themeSurface
    palette.highlight: activeAppearance.surfaces.selection
    palette.highlightedText: activeAppearance.foregrounds["selected-text"]
    background: Rectangle {
        gradient: Gradient {
            GradientStop { position: 0; color: activeAppearance.surfaces.b0 }
            GradientStop { position: 0.7; color: activeAppearance.surfaces.b1 }
            GradientStop { position: 1; color: activeAppearance.surfaces.b1 }
        }
    }
    Component.onCompleted: {
        const geometry = geometryCommands.itemAt(0).info
        windowPolicy.savedSize = Qt.size(geometry.width, geometry.height)
        windowPolicy.attach(window)
        launcher.window = window
        if (!launcherStartHidden) window.show()
        Qt.callLater(window.startShortcutSetup)
    }
    function requestVisibility(command) { visibilityCommand.itemAt(0).request(command) }
    Repeater {
        id: visibilityCommand
        model: launcherCommands
        delegate: Item { function request(command) { model.command = command } }
    }
    Shortcut {
        sequence: "Escape"
        enabled: !window.shortcutInfo.recording
        onActivated: {
            if (window.shortcutInfo.recording) return
            if (notifications.item && notifications.item.closeFocusedDetails()) return
            if (window.settingsOpen && !window.onboarding) {
                if (window.integrationOpen) {
                    window.integrationOpen = false
                    window.integrationStartupResolved = true
                    window.startShortcutSetup()
                } else window.backFromSettings()
                return
            }
            window.requestVisibility("hide")
        }
    }
    Repeater {
        id: geometryCommands
        model: windowGeometry
        delegate: Item {
            readonly property var info: JSON.parse(model.snapshot)
            property size rememberedSize: Qt.size(info.width, info.height)
            property bool placementPending: false
            function request(action) {
                model.command = JSON.stringify({action: action,
                    width: action === "reset" ? 640 : action === "place"
                        ? windowPolicy.constrainedSize.width : window.width,
                    height: action === "reset" ? 420 : action === "place"
                        ? windowPolicy.constrainedSize.height : window.height,
                    visible: window.visible})
            }
            onInfoChanged: {
                if (info.reset > 0) {
                    rememberedSize = Qt.size(640, 420)
                    windowPolicy.savedSize = Qt.size(640, 420)
                    windowPolicy.applySize()
                }
            }
            Connections {
                target: window
                function onVisibleChanged() {
                    if (!window.visible) return
                    windowPolicy.savedSize = rememberedSize
                    windowPolicy.applySize()
                    placementPending = true
                }
                // Before mapping, QWindow can still report the screen's rounded DPR.
                function onFrameSwapped() {
                    if (!placementPending || !window.visible) return
                    placementPending = false
                    request("place")
                }
            }
            Timer {
                interval: 500
                running: window.visible && !placementPending
                repeat: true
                onTriggered: {
                    rememberedSize = Qt.size(window.width, window.height)
                    request("observe")
                }
            }
        }
    }
    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 16
        spacing: 8
        RowLayout {
            visible: window.settingsOpen || window.integrationOpen || window.onboarding
            Label {
                text: window.onboarding ? "Shortcut setup" : window.settingsOpen ? "Settings" : "Integration"
                font.pixelSize: 20
                Layout.fillWidth: true
                MouseArea { anchors.fill: parent; onPressed: windowPolicy.startMove() }
            }
            Action {
                text: "Back"
                id: backButton
                objectName: "settings-back"
                visible: window.settingsOpen && !window.integrationOpen && !window.onboarding
                onClicked: window.backFromSettings()
            }
            Action {
                id: integrationButton
                objectName: "settings-integration"
                text: window.integrationOpen ? "Back" : "Integration"
                visible: window.integrationOpen || window.settingsOpen || window.onboarding
                onClicked: {
                    if (window.integrationOpen) {
                        window.integrationOpen = false
                        window.integrationStartupResolved = true
                        window.startShortcutSetup()
                    } else window.openIntegration()
                }
            }
        }
        RowLayout {
            visible: window.settingsOpen && !window.onboarding && !window.integrationOpen
            Layout.fillWidth: true
            Action {
                objectName: "settings-appearance"
                text: "Appearance"
                primary: window.settingsTab === "appearance"
                onClicked: window.settingsTab = "appearance"
            }
            Action {
                objectName: "settings-shortcut"
                text: "Keyboard shortcut"
                primary: window.settingsTab === "shortcut"
                onClicked: window.settingsTab = "shortcut"
            }
            Item { Layout.fillWidth: true }
            Action {
                objectName: "settings-window-reset"
                text: "Reset window layout"
                onClicked: geometryCommands.itemAt(0).request("reset")
            }
        }
        Loader {
            id: appearanceSettings
            Layout.fillWidth: true
            Layout.fillHeight: true
            visible: window.settingsOpen && !window.onboarding && !window.integrationOpen && window.settingsTab === "appearance"
            sourceComponent: visible ? runixAppearanceSettingsComponent : null
        }
        Repeater {
            id: integrationPanel
            model: integrationState
            delegate: ColumnLayout {
                id: setup
                Layout.fillWidth: true
                Layout.fillHeight: true
                visible: window.integrationOpen
                readonly property var info: JSON.parse(model.snapshot)
                property bool copied: false
                readonly property bool needsInstall: info.module_state === "missing" || info.module_state === "stale"
                function request(action) {
                    model.command = JSON.stringify({action: action, automatic: automatic.item ? automatic.item.checked : false})
                }
                onInfoChanged: {
                    if (!window.integrationStartupResolved) {
                        window.integrationOpen = info.phase !== "ready" && info.phase !== "unavailable"
                        if (!info.busy && !window.integrationOpen) {
                            window.integrationStartupResolved = true
                            Qt.callLater(window.startShortcutSetup)
                        }
                    }
                    if (window.shortcutStarted && !info.busy) window.shortcutRequest("refresh")
                }
                ScrollView {
                    id: setupScroll
                    ScrollBar.vertical: runixScrollBarComponent.createObject(null)
                    background: Item {}
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    clip: true
                    contentWidth: availableWidth
                    contentHeight: setupContent.implicitHeight + 8
                    ColumnLayout {
                        id: setupContent
                        x: 4
                        y: 4
                        width: Math.max(0, setupScroll.availableWidth - 8)
                        spacing: 16
                        RowLayout {
                            Layout.fillWidth: true
                            Label {
                                text: "Hyprland integration"
                                font.pixelSize: 18
                                font.bold: true
                                Layout.fillWidth: true
                            }
                            Label {
                                text: setup.info.busy ? "Checking…" : setup.info.available ? "Connected" : "Not connected"
                                color: setup.info.busy ? runixControlStyle.loader : setup.info.available
                                    ? runixControlStyle.hueText("green") : runixControlStyle.secondary
                                font.pixelSize: 12
                            }
                        }
                        Notice {
                            text: setup.info.status || ""
                            hue: setup.info.phase === "error" ? "red" : setup.info.available ? "green" : "blue"
                        }
                        Pane {
                            Layout.fillWidth: true
                            visible: !!setup.info.path
                            padding: 16
                            background: Item {}
                            ColumnLayout {
                                anchors.fill: parent
                                spacing: 8
                                RowLayout {
                                    Layout.fillWidth: true
                                    Label {
                                        text: "1. Install module"
                                        font.bold: true
                                        Layout.fillWidth: true
                                    }
                                    Label {
                                        visible: setup.info.module_state === "current"
                                        text: "Current version installed"
                                        color: runixControlStyle.hueText("green")
                                    }
                                    Action {
                                        objectName: "installIntegration"
                                        text: setup.info.module_state === "missing" ? "Install" : "Update"
                                        visible: setup.needsInstall
                                        enabled: !setup.info.busy
                                        busy: setup.info.busy
                                        onClicked: setup.request("install")
                                    }
                                }
                                Label {
                                    Layout.fillWidth: true
                                    text: setup.info.path || ""
                                    textFormat: Text.PlainText
                                    font.family: "monospace"
                                    font.pixelSize: 12
                                    color: runixControlStyle.secondary
                                    wrapMode: Text.WrapAnywhere
                                }
                            }
                        }
                        Pane {
                            Layout.fillWidth: true
                            visible: !!setup.info.require && !setup.info.available
                            padding: 16
                            background: Item {}
                            ColumnLayout {
                                anchors.fill: parent
                                spacing: 10
                                RowLayout {
                                    Layout.fillWidth: true
                                    Label {
                                        text: "2. Connect to Hyprland"
                                        font.bold: true
                                        Layout.fillWidth: true
                                    }
                                    Action {
                                        objectName: "copyIntegration"
                                        text: setup.copied ? "Copied" : "Copy"
                                        onClicked: {
                                            requireCode.item.selectAll()
                                            requireCode.item.copy()
                                            requireCode.item.deselect()
                                            setup.copied = true
                                            copiedTimer.restart()
                                        }
                                    }
                                }
                                Label {
                                    Layout.fillWidth: true
                                    text: "Add this line to hyprland.lua. Then click Reload and check."
                                    color: runixControlStyle.secondary
                                    wrapMode: Text.Wrap
                                }
                                Loader {
                                    id: requireCode
                                    Layout.fillWidth: true
                                    sourceComponent: runixTextAreaComponent
                                    onLoaded: {
                                        item.objectName = "integrationCode"
                                        item.text = Qt.binding(function() { return setup.info.require || "" })
                                        item.textFormat = TextEdit.PlainText
                                        item.font.family = "monospace"
                                        item.font.pixelSize = 12
                                        item.readOnly = true
                                        item.wrapMode = TextEdit.WrapAnywhere
                                        item.textChanged.connect(function() { setup.copied = false })
                                    }
                                }
                                Action {
                                    text: "Reload and check"
                                    enabled: !setup.info.busy && setup.info.module_state === "current"
                                    onClicked: setup.request("confirm")
                                }
                            }
                        }
                        Loader {
                            id: automatic
                            Layout.fillWidth: true
                            visible: setup.info.phase !== "unavailable"
                            enabled: !setup.info.busy
                            sourceComponent: runixCheckBoxComponent
                            onLoaded: {
                                item.text = "Install and update the module at startup"
                                item.checked = Qt.binding(function() { return !!setup.info.automatic })
                                item.toggled.connect(function() { setup.request("preference") })
                            }
                        }
                        Label {
                            Layout.fillWidth: true
                            visible: setup.info.phase !== "unavailable"
                            text: "Updates and connection checks reload Hyprland. Runix never changes your Hyprland configuration."
                            font.pixelSize: 12
                            color: runixControlStyle.secondary
                            wrapMode: Text.Wrap
                        }
                    }
                }
                RowLayout {
                    Layout.fillWidth: true
                    Action {
                        text: "Check again"
                        enabled: !setup.info.busy
                        onClicked: setup.request("refresh")
                    }
                    Item { Layout.fillWidth: true }
                    Action {
                        text: setup.info.available ? "Continue" : "Not now"
                        primary: true
                        onClicked: {
                            window.integrationStartupResolved = true
                            window.integrationOpen = false
                            window.startShortcutSetup()
                            if (window.shortcutStarted) window.shortcutRequest("refresh")
                        }
                    }
                }
                Timer {
                    id: copiedTimer
                    interval: 2000
                    onTriggered: setup.copied = false
                }
            }
        }
        Repeater {
            id: shortcutPanel
            model: shortcutState
            delegate: ScrollView {
                id: shortcuts
                ScrollBar.vertical: runixScrollBarComponent.createObject(null)
                background: Item {}
                Layout.fillWidth: true
                Layout.fillHeight: true
                visible: window.settingsOpen && !window.integrationOpen && (window.onboarding || window.settingsTab === "shortcut")
                clip: true
                contentWidth: availableWidth
                contentHeight: shortcutContent.implicitHeight + 8
                readonly property var info: JSON.parse(model.snapshot)
                readonly property var focusControl: skipShortcut.item
                function request(action) { model.command = JSON.stringify({action: action}) }
                function combination(value) {
                    if (!value) return "Not assigned"
                    let parts = []
                    if (value.modifiers & 4) parts.push("Ctrl")
                    if (value.modifiers & 8) parts.push("Alt")
                    if (value.modifiers & 1) parts.push("Shift")
                    if (value.modifiers & 64) parts.push("Super")
                    parts.push("Keycode " + value.keycode)
                    return parts.join(" + ")
                }
                function savedStatus(value) {
                    switch (value) {
                    case "active": return "Ready to use"
                    case "inactive": return "Saved, but not active"
                    case "unassigned": return "No shortcut saved"
                    case "conflict": return "Also used by another shortcut"
                    case "degraded": return "Could not check the shortcut"
                    default: return "Checking..."
                    }
                }
                function candidateStatus(value) {
                    switch (value) {
                    case "owned": return "Assigned to runix"
                    case "free": return "Available"
                    case "foreign":
                    case "conflict": return "Already in use"
                    default: return "Could not check this key combination"
                    }
                }
                onInfoChanged: {
                    const wasOnboarding = window.onboarding
                    window.shortcutInfo = info
                    if (window.shortcutStarted && !info.busy) window.shortcutStartupResolved = true
                    window.onboarding = !!info.onboarding
                    if (window.onboarding) window.settingsOpen = true
                    if (info.completed && !info.onboarding && wasOnboarding) {
                        window.settingsOpen = false
                    }
                }
                ColumnLayout {
                    id: shortcutContent
                    x: 4
                    y: 4
                    width: Math.max(0, shortcuts.availableWidth - 8)
                    spacing: 8
                    Label {
                        text: window.onboarding ? "Set up your shortcut" : "Keyboard shortcut"
                        font.pixelSize: 18
                    }
                    Label {
                        Layout.fillWidth: true
                        text: "Show or hide runix"
                        font.bold: true
                    }
                    Label {
                        Layout.fillWidth: true
                        text: "Saved: " + shortcuts.combination(shortcuts.info.saved)
                            + " · " + shortcuts.savedStatus(shortcuts.info.status)
                        textFormat: Text.PlainText
                        wrapMode: Text.Wrap
                    }
                    Notice {
                        text: shortcuts.info.error || ""
                        hue: "red"
                    }
                    Label {
                        Layout.fillWidth: true
                        visible: !shortcuts.info.available
                        text: "To set a shortcut, connect the Hyprland module in Integration. "
                            + "You can also continue without a shortcut."
                        wrapMode: Text.Wrap
                    }
                    Pane {
                        Layout.fillWidth: true
                        padding: 10
                        background: Item {
                            Rectangle {
                                anchors.fill: parent
                                radius: runixControlStyle.radius
                                color: runixControlStyle.button
                                border.width: 1
                                border.color: shortcuts.info.recorder_error || shortcuts.info.candidate_error
                                    ? runixControlStyle.error : runixControlStyle.separator
                            }
                            Rectangle {
                                anchors.fill: parent
                                anchors.margins: -2
                                radius: runixControlStyle.radius + 2
                                color: "transparent"
                                border.width: shortcuts.info.recording ? 2 : 0
                                border.color: runixControlStyle.focus
                            }
                        }
                        Label {
                            width: parent.width
                            text: shortcuts.info.recording ? "Press a key combination. Press Escape to cancel."
                                : shortcuts.info.candidate ? "New shortcut: " + shortcuts.combination(shortcuts.info.candidate)
                                    + " · " + shortcuts.candidateStatus(shortcuts.info.candidate_status)
                                : "Press Record shortcut, then press a key. You can hold Ctrl, Alt, Shift, or Super."
                            textFormat: Text.PlainText
                            color: runixControlStyle.text
                            wrapMode: Text.Wrap
                        }
                    }
                    Notice {
                        text: shortcuts.info.recorder_error || shortcuts.info.candidate_error || ""
                        hue: "red"
                    }
                    Label {
                        Layout.fillWidth: true
                        text: "A keycode is the number of a physical key. It stays the same when you change keyboard layouts."
                        color: runixControlStyle.secondary
                        wrapMode: Text.Wrap
                    }
                    Flow {
                        Layout.fillWidth: true
                        spacing: 8
                        Action {
                            text: shortcuts.info.recording ? "Cancel recording" : "Record shortcut"
                            enabled: shortcuts.info.recording || (!shortcuts.info.busy && shortcuts.info.available)
                            onClicked: shortcuts.request(shortcuts.info.recording ? "cancel" : "record")
                        }
                        Action {
                            text: window.onboarding ? "Save and continue" : "Save"
                            primary: true
                            busy: shortcuts.info.busy
                            enabled: !shortcuts.info.busy && !shortcuts.info.recording && shortcuts.info.can_save
                            onClicked: shortcuts.request("apply")
                        }
                        Action {
                            text: "Remove"
                            destructive: true
                            visible: !!shortcuts.info.saved
                            enabled: !shortcuts.info.busy && !shortcuts.info.recording && shortcuts.info.available
                            onClicked: shortcuts.request("remove")
                        }
                        Action {
                            text: "Check again"
                            enabled: !shortcuts.info.busy && !shortcuts.info.recording
                            onClicked: {
                                integrationPanel.itemAt(0).request("refresh")
                                shortcuts.request("refresh")
                            }
                        }
                        Action { text: "Integration"; onClicked: window.openIntegration() }
                        Action {
                            id: skipShortcut
                            text: window.onboarding ? "Not now" : "Back to applications"
                            enabled: !window.onboarding || !shortcuts.info.busy
                            onClicked: {
                                shortcuts.request("cancel")
                                if (window.onboarding) shortcuts.request("skip")
                                else window.settingsOpen = false
                            }
                        }
                    }
                    Label {
                        visible: !!shortcuts.info.busy
                        text: "◌ Checking or saving shortcut..."
                        color: runixControlStyle.loader
                    }
                }
            }
        }
        Repeater {
            model: catalogState
            delegate: ColumnLayout {
                id: catalog
                visible: !window.integrationOpen && !window.settingsOpen && window.shortcutStartupResolved
                Layout.fillWidth: true
                Layout.fillHeight: true
                readonly property bool loading: model.loading
                readonly property bool unavailable: model.unavailable
                Repeater {
                    model: searchState
                    delegate: ColumnLayout {
                        id: search
                        objectName: "search-state"
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        readonly property var info: JSON.parse(model.snapshot)
                        Item {
                            Layout.fillWidth: true
                            implicitHeight: runixControlStyle.spacing
                            MouseArea {
                                anchors.fill: parent
                                onPressed: windowPolicy.startMove()
                            }
                        }
                        function resetSearch() { model.command = JSON.stringify({op: "reset"}) }
                        Loader {
                            id: query
                            objectName: "search-query"
                            Layout.fillWidth: true
                            sourceComponent: runixTextFieldComponent
                            onLoaded: {
                                item.search = true
                                item.placeholderText = "Search applications and commands…"
                                item.Accessible.name = "Search applications and commands"
                                item.text = Qt.binding(function() { return search.info.query || "" })
                                item.textEdited.connect(function() {
                                    model.command = JSON.stringify({op: "query", text: item.text})
                                })
                                item.Keys.upPressed.connect(function(event) {
                                    if (results.item) results.item.move(-1)
                                    event.accepted = true
                                })
                                item.Keys.downPressed.connect(function(event) {
                                    if (results.item) results.item.move(1)
                                    event.accepted = true
                                })
                                function activate(event) {
                                    if (!event.isAutoRepeat && results.item) results.item.activate()
                                    event.accepted = true
                                }
                                item.Keys.returnPressed.connect(activate)
                                item.Keys.enterPressed.connect(activate)
                                item.Keys.pressed.connect(function(event) {
                                    if (!results.item) return
                                    if (event.key === Qt.Key_PageDown || event.key === Qt.Key_PageUp) {
                                        results.item.move(event.key === Qt.Key_PageDown ? 6 : -6)
                                        event.accepted = true
                                    }
                                })
                                updateFocus()
                            }
                            function updateFocus() {
                                launcher.query = window.integrationOpen ? integrationButton.item
                                    : window.settingsOpen ? (window.onboarding && shortcutPanel.itemAt(0)
                                        ? shortcutPanel.itemAt(0).focusControl : backButton.item) : item
                                if (window.visible && launcher.query) launcher.query.forceActiveFocus()
                            }
                            Connections {
                                target: window
                                function onSearchResetSignalChanged() { search.resetSearch() }
                                function onIntegrationOpenChanged() { query.updateFocus() }
                                function onSettingsOpenChanged() { query.updateFocus() }
                                function onOnboardingChanged() { query.updateFocus() }
                            }
                        }
                        Loader {
                            id: results
                            objectName: "search-results"
                            Layout.fillWidth: true
                            Layout.fillHeight: true
                            sourceComponent: resultListComponent
                            onLoaded: {
                                item.state = model
                                item.providerLoading = Qt.binding(function() { return catalog.loading })
                                item.providerUnavailable = Qt.binding(function() { return catalog.unavailable })
                            }
                        }
                        Rectangle {
                            Layout.fillWidth: true
                            implicitHeight: 1
                            color: runixControlStyle.separator
                        }
                        RowLayout {
                            Layout.fillWidth: true
                            implicitHeight: 36
                            Label {
                                text: window.launchInfo.busy ? "Opening…" : ""
                                color: runixControlStyle.loader
                                textFormat: Text.PlainText
                                Layout.fillWidth: true
                            }
                            Button {
                                id: primaryAction
                                objectName: "search-primary-action"
                                text: search.info.primary_action || "Open item"
                                Accessible.name: text
                                enabled: !!search.info.activation_enabled && !search.info.loading && search.info.selected_index >= 0
                                focusPolicy: Qt.StrongFocus
                                hoverEnabled: true
                                padding: runixControlStyle.spacing
                                contentItem: Row {
                                    spacing: runixControlStyle.spacing
                                    Label {
                                        text: primaryAction.text
                                        textFormat: Text.PlainText
                                        font.bold: true
                                        color: runixControlStyle.foreground(primaryAction)
                                        anchors.verticalCenter: parent.verticalCenter
                                    }
                                    Rectangle {
                                        width: 24
                                        height: 22
                                        radius: runixControlStyle.radius
                                        color: runixControlStyle.button
                                        Label {
                                            anchors.centerIn: parent
                                            text: "↵"
                                            color: runixControlStyle.foreground(primaryAction)
                                        }
                                    }
                                }
                                background: Rectangle {
                                    radius: runixControlStyle.radius
                                    color: runixControlStyle.wash(actionPointer)
                                    border.width: primaryAction.visualFocus ? 2 : 0
                                    border.color: runixControlStyle.focus
                                }
                                onClicked: { if (results.item) results.item.activate() }
                                Keys.onReturnPressed: function(event) {
                                    if (!event.isAutoRepeat && results.item) results.item.activate()
                                }
                                Keys.onEnterPressed: function(event) {
                                    if (!event.isAutoRepeat && results.item) results.item.activate()
                                }
                                MouseArea {
                                    id: actionPointer
                                    anchors.fill: parent
                                    acceptedButtons: Qt.LeftButton
                                    hoverEnabled: true
                                    property var target: null
                                    onPressed: target = results.item ? results.item.activationTarget() : null
                                    onClicked: { if (results.item && target) results.item.activate(target) }
                                    onDoubleClicked: function(event) { event.accepted = true }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    Loader {
        id: notifications
        anchors.fill: parent
        z: 100
        sourceComponent: runixNotificationsComponent
        onLoaded: {
            item.hostWindow = window
            item.searchControl = Qt.binding(function() { return launcher.query })
            item.recording = Qt.binding(function() { return !!window.shortcutInfo.recording })
        }
    }
}
