import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ApplicationWindow {
    id: window
    visible: false
    title: "runix"
    flags: Qt.Window | Qt.FramelessWindowHint
    onClosing: Qt.quit()
    property var launchInfo: ({busy: false, status: ""})
    property bool integrationOpen: false
    property bool integrationStartupResolved: false
    property bool shortcutStarted: false
    property bool shortcutStartupResolved: false
    property bool settingsOpen: false
    property bool onboarding: false
    property var shortcutInfo: ({busy: true, onboarding: false, available: false})
    property string shortcutNotice: ""
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
    onSettingsOpenChanged: { if (!settingsOpen) shortcutRequest("cancel") }
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
    readonly property color themeForeground: "#e8edf2"
    readonly property color themeSurface: "#202832"
    palette.window: themeSurface
    palette.windowText: themeForeground
    palette.text: themeForeground
    palette.buttonText: themeForeground
    palette.base: "#151c23"
    palette.button: themeSurface
    palette.highlight: "#38618a"
    Component.onCompleted: {
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
            if (window.settingsOpen && !window.onboarding) { window.settingsOpen = false; return }
            window.requestVisibility("hide")
        }
    }
    Repeater {
        model: windowCommands
        delegate: Item {
            function requestSize() {
                if (windowPolicy.attached && window.visible)
                    model.request = windowPolicy.constrainedSize
            }
            Connections {
                target: windowPolicy
                function onConstrainedSizeChanged() { requestSize() }
                function onAttachedChanged() { requestSize() }
            }
            Connections {
                target: window
                function onVisibleChanged() { requestSize() }
                function onDevicePixelRatioChanged() { requestSize() }
            }
        }
    }
    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 16
        spacing: 8
        RowLayout {
            Label {
                text: window.onboarding ? "Shortcut setup" : window.settingsOpen ? "Settings" : window.integrationOpen ? "Integration" : "Applications"
                font.pixelSize: 20
                Layout.fillWidth: true
                MouseArea { anchors.fill: parent; onPressed: windowPolicy.startMove() }
            }
            Button {
                id: settingsButton
                text: window.settingsOpen ? "Back" : "Settings"
                visible: !window.integrationOpen && !window.onboarding
                onClicked: {
                    if (window.settingsOpen) window.settingsOpen = false
                    else {
                        window.settingsOpen = true
                        integrationPanel.itemAt(0).request("refresh")
                        window.shortcutRequest("refresh")
                    }
                }
            }
            Button {
                id: integrationButton
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
            Button { text: "Quit"; onClicked: Qt.quit() }
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
                    model.command = JSON.stringify({action: action, automatic: automatic.checked})
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
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    clip: true
                    contentWidth: availableWidth
                    ColumnLayout {
                        width: setupScroll.availableWidth
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
                                color: setup.info.available ? "#93d5aa" : "#aebcca"
                                font.pixelSize: 12
                            }
                        }
                        Label {
                            Layout.fillWidth: true
                            text: setup.info.status || ""
                            textFormat: Text.PlainText
                            color: setup.info.phase === "error" ? "#ff8a80" : "#aebcca"
                            wrapMode: Text.Wrap
                        }
                        Pane {
                            Layout.fillWidth: true
                            visible: !!setup.info.path
                            padding: 16
                            background: Rectangle { color: "#151c23"; radius: 8; border.color: "#34414e" }
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
                                        text: "Up to date"
                                        color: "#93d5aa"
                                    }
                                    Button {
                                        objectName: "installIntegration"
                                        text: setup.info.module_state === "missing" ? "Install" : "Update"
                                        visible: setup.needsInstall
                                        enabled: !setup.info.busy
                                        onClicked: setup.request("install")
                                    }
                                }
                                Label {
                                    Layout.fillWidth: true
                                    text: setup.info.path || ""
                                    textFormat: Text.PlainText
                                    font.family: "monospace"
                                    font.pixelSize: 12
                                    color: "#aebcca"
                                    wrapMode: Text.WrapAnywhere
                                }
                            }
                        }
                        Pane {
                            Layout.fillWidth: true
                            visible: !!setup.info.require && !setup.info.available
                            padding: 16
                            background: Rectangle { color: "#151c23"; radius: 8; border.color: "#34414e" }
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
                                    Button {
                                        objectName: "copyIntegration"
                                        text: setup.copied ? "Copied" : "Copy"
                                        onClicked: {
                                            requireCode.selectAll()
                                            requireCode.copy()
                                            requireCode.deselect()
                                            setup.copied = true
                                            copiedTimer.restart()
                                        }
                                    }
                                }
                                Label {
                                    Layout.fillWidth: true
                                    text: "Add this line to hyprland.lua, then reload and check the connection."
                                    color: "#aebcca"
                                    wrapMode: Text.Wrap
                                }
                                TextArea {
                                    id: requireCode
                                    objectName: "integrationCode"
                                    Layout.fillWidth: true
                                    text: setup.info.require || ""
                                    textFormat: TextEdit.PlainText
                                    font.family: "monospace"
                                    font.pixelSize: 12
                                    readOnly: true
                                    selectByMouse: true
                                    wrapMode: TextEdit.WrapAnywhere
                                    onTextChanged: setup.copied = false
                                }
                                Button {
                                    text: "Reload and check"
                                    enabled: !setup.info.busy && setup.info.module_state === "current"
                                    onClicked: setup.request("confirm")
                                }
                            }
                        }
                        CheckBox {
                            id: automatic
                            Layout.fillWidth: true
                            visible: setup.info.phase !== "unavailable"
                            enabled: !setup.info.busy
                            text: "Install and update automatically at startup"
                            checked: !!setup.info.automatic
                            onToggled: setup.request("preference")
                        }
                        Label {
                            Layout.fillWidth: true
                            visible: setup.info.phase !== "unavailable"
                            text: "Updates and connection checks reload Hyprland. Runix never edits your Hyprland config."
                            font.pixelSize: 12
                            color: "#aebcca"
                            wrapMode: Text.Wrap
                        }
                    }
                }
                RowLayout {
                    Layout.fillWidth: true
                    Button {
                        text: "Check again"
                        enabled: !setup.info.busy
                        onClicked: setup.request("refresh")
                    }
                    Item { Layout.fillWidth: true }
                    Button {
                        text: setup.info.available ? "Continue" : "Not now"
                        highlighted: true
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
                Layout.fillWidth: true
                Layout.fillHeight: true
                visible: window.settingsOpen && !window.integrationOpen
                clip: true
                contentWidth: availableWidth
                readonly property var info: JSON.parse(model.snapshot)
                readonly property var focusControl: skipShortcut
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
                onInfoChanged: {
                    const wasOnboarding = window.onboarding
                    window.shortcutInfo = info
                    if (window.shortcutStarted && !info.busy) window.shortcutStartupResolved = true
                    window.onboarding = !!info.onboarding
                    if (window.onboarding) window.settingsOpen = true
                    if (info.completed && !info.onboarding && wasOnboarding) {
                        window.settingsOpen = false
                        if (info.error) window.shortcutNotice = info.error
                    }
                }
                ColumnLayout {
                    width: shortcuts.availableWidth
                    spacing: 8
                    Label {
                        text: window.onboarding ? "Set up your shortcut" : "Keyboard shortcut"
                        font.pixelSize: 18
                    }
                    Label {
                        Layout.fillWidth: true
                        text: "Toggle runix"
                        font.bold: true
                    }
                    Label {
                        Layout.fillWidth: true
                        text: "Saved: " + shortcuts.combination(shortcuts.info.saved)
                            + " · " + (shortcuts.info.status || "Checking...")
                        textFormat: Text.PlainText
                        wrapMode: Text.Wrap
                    }
                    Label {
                        Layout.fillWidth: true
                        visible: !!shortcuts.info.error
                        text: shortcuts.info.error || ""
                        textFormat: Text.PlainText
                        color: "#ff8a80"
                        wrapMode: Text.Wrap
                    }
                    Label {
                        Layout.fillWidth: true
                        visible: !shortcuts.info.available
                        text: "Shortcut assignment needs a ready Hyprland module. Open Integration to repair it, "
                            + "or continue without a shortcut."
                        wrapMode: Text.Wrap
                    }
                    Label {
                        Layout.fillWidth: true
                        text: shortcuts.info.recording ? "Press a shortcut. Escape cancels."
                            : shortcuts.info.candidate ? "Candidate: " + shortcuts.combination(shortcuts.info.candidate)
                                + " · " + (shortcuts.info.candidate_status === "owned"
                                    ? "Owned by runix: toggle" : shortcuts.info.candidate_status)
                            : "Record a key with optional Ctrl, Alt, Shift, or Super."
                        textFormat: Text.PlainText
                        wrapMode: Text.Wrap
                    }
                    Label {
                        Layout.fillWidth: true
                        visible: !!shortcuts.info.recorder_error || !!shortcuts.info.candidate_error
                        text: shortcuts.info.recorder_error || shortcuts.info.candidate_error || ""
                        textFormat: Text.PlainText
                        color: "#ff8a80"
                        wrapMode: Text.Wrap
                    }
                    Label {
                        Layout.fillWidth: true
                        text: "Keycode identifies a physical key and does not change with the keyboard layout."
                        wrapMode: Text.Wrap
                    }
                    Flow {
                        Layout.fillWidth: true
                        spacing: 8
                        Button {
                            text: shortcuts.info.recording ? "Cancel recording" : "Record shortcut"
                            enabled: shortcuts.info.recording || (!shortcuts.info.busy && shortcuts.info.available)
                            onClicked: shortcuts.request(shortcuts.info.recording ? "cancel" : "record")
                        }
                        Button {
                            text: window.onboarding ? "Save and continue" : "Save"
                            enabled: !shortcuts.info.busy && !shortcuts.info.recording && shortcuts.info.can_save
                            onClicked: shortcuts.request("apply")
                        }
                        Button {
                            text: "Remove"
                            visible: !!shortcuts.info.saved
                            enabled: !shortcuts.info.busy && !shortcuts.info.recording && shortcuts.info.available
                            onClicked: shortcuts.request("remove")
                        }
                        Button {
                            text: "Check again"
                            enabled: !shortcuts.info.busy && !shortcuts.info.recording
                            onClicked: {
                                integrationPanel.itemAt(0).request("refresh")
                                shortcuts.request("refresh")
                            }
                        }
                        Button { text: "Integration"; onClicked: window.openIntegration() }
                        Button {
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
                    Label { visible: !!shortcuts.info.busy; text: "Checking or saving shortcut..." }
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
                readonly property bool failed: model.failed
                Label {
                    Layout.fillWidth: true
                    visible: window.shortcutNotice !== ""
                    text: window.shortcutNotice
                    textFormat: Text.PlainText
                    wrapMode: Text.Wrap
                    color: "#ff8a80"
                }
                Label {
                    Layout.fillWidth: true
                    text: model.status
                    textFormat: Text.PlainText
                    wrapMode: Text.Wrap
                }
                Label {
                    Layout.fillWidth: true
                    visible: model.activation_error !== ""
                    text: model.activation_error
                    textFormat: Text.PlainText
                    wrapMode: Text.Wrap
                    color: "#ff8a80"
                }
                Label {
                    Layout.fillWidth: true
                    visible: model.history_warning !== ""
                    text: model.history_warning
                    textFormat: Text.PlainText
                    wrapMode: Text.Wrap
                    color: "#f0c674"
                }
                Repeater {
                    model: searchState
                    delegate: ColumnLayout {
                        id: search
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        readonly property var info: JSON.parse(model.snapshot)
                        TextField {
                            id: query
                            Layout.fillWidth: true
                            color: window.palette.text
                            placeholderTextColor: "#aab6c2"
                            placeholderText: "Search applications"
                            selectByMouse: true
                            text: search.info.query || ""
                            onTextEdited: model.command = JSON.stringify({op: "query", text: text})
                            function updateFocus() {
                                launcher.query = window.integrationOpen ? integrationButton
                                    : window.settingsOpen ? (window.onboarding && shortcutPanel.itemAt(0)
                                        ? shortcutPanel.itemAt(0).focusControl : settingsButton) : query
                                if (window.visible && launcher.query) launcher.query.forceActiveFocus()
                            }
                            Component.onCompleted: updateFocus()
                            Connections {
                                target: window
                                function onIntegrationOpenChanged() { query.updateFocus() }
                                function onSettingsOpenChanged() { query.updateFocus() }
                                function onOnboardingChanged() { query.updateFocus() }
                            }
                            Keys.onUpPressed: { if (results.item) results.item.move(-1) }
                            Keys.onDownPressed: { if (results.item) results.item.move(1) }
                            Keys.onReturnPressed: function(event) {
                                if (!event.isAutoRepeat && results.item) results.item.activate()
                            }
                            Keys.onEnterPressed: function(event) {
                                if (!event.isAutoRepeat && results.item) results.item.activate()
                            }
                            Keys.onPressed: function(event) {
                                if (!results.item) return
                                if (event.key === Qt.Key_PageDown || event.key === Qt.Key_PageUp) {
                                    results.item.move(event.key === Qt.Key_PageDown ? 6 : -6)
                                    event.accepted = true
                                }
                            }
                        }
                        Loader {
                            id: results
                            Layout.fillWidth: true
                            Layout.fillHeight: true
                            visible: !catalog.loading && !catalog.failed
                            sourceComponent: resultListComponent
                            onLoaded: item.state = model
                        }
                        Item {
                            Layout.fillWidth: true
                            Layout.fillHeight: true
                            visible: catalog.loading || catalog.failed
                        }
                        Label {
                            Layout.fillWidth: true
                            text: search.info.selected_label || ""
                            textFormat: Text.PlainText
                            wrapMode: Text.WrapAnywhere
                        }
                        RowLayout {
                            Button {
                                text: "Launch"
                                enabled: search.info.activation_enabled && !search.info.loading && search.info.selected_index >= 0
                                onClicked: { if (results.item) results.item.activate() }
                            }
                            Label {
                                Layout.fillWidth: true
                                text: window.launchInfo.status
                                textFormat: Text.PlainText
                                wrapMode: Text.Wrap
                            }
                        }
                    }
                }
            }
        }
    }
}
