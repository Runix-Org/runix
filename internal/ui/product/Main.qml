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
    }
    function requestVisibility(command) { visibilityCommand.itemAt(0).request(command) }
    Repeater {
        id: visibilityCommand
        model: launcherCommands
        delegate: Item { function request(command) { model.command = command } }
    }
    Shortcut { sequence: "Escape"; onActivated: window.requestVisibility("hide") }
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
                text: "Applications"
                font.pixelSize: 20
                Layout.fillWidth: true
                MouseArea { anchors.fill: parent; onPressed: windowPolicy.startMove() }
            }
            Button {
                id: integrationButton
                text: "Integration"
                onClicked: {
                    window.integrationStartupResolved = true
                    window.integrationOpen = !window.integrationOpen
                    if (window.integrationOpen) integrationPanel.itemAt(0).request("refresh")
                }
            }
            Button { text: "Quit"; onClicked: Qt.quit() }
        }
        Repeater {
            id: integrationPanel
            model: integrationState
            delegate: ScrollView {
                id: setup
                Layout.fillWidth: true
                Layout.fillHeight: true
                visible: window.integrationOpen
                clip: true
                contentWidth: availableWidth
                readonly property var info: JSON.parse(model.snapshot)
                function request(action) {
                    model.command = JSON.stringify({action: action, automatic: automatic.checked})
                }
                onInfoChanged: {
                    if (!window.integrationStartupResolved) {
                        window.integrationOpen = info.phase !== "ready" && info.phase !== "unavailable"
                        if (!info.busy) window.integrationStartupResolved = true
                    }
                }
                ColumnLayout {
                    width: setup.availableWidth
                    spacing: 10
                    Label {
                        text: "Hyprland integration"
                        font.pixelSize: 18
                    }
                    Label {
                        Layout.fillWidth: true
                        text: setup.info.status || ""
                        textFormat: Text.PlainText
                        wrapMode: Text.Wrap
                    }
                    Label {
                        Layout.fillWidth: true
                        visible: !!setup.info.path
                        text: setup.info.path || ""
                        textFormat: Text.PlainText
                        wrapMode: Text.WrapAnywhere
                    }
                    Label {
                        Layout.fillWidth: true
                        visible: !!setup.info.require && !setup.info.available
                        text: "Add this line to your Hyprland Lua config. Then click I connected it."
                        textFormat: Text.PlainText
                        wrapMode: Text.Wrap
                    }
                    TextArea {
                        Layout.fillWidth: true
                        visible: !!setup.info.require && !setup.info.available
                        text: setup.info.require || ""
                        textFormat: TextEdit.PlainText
                        readOnly: true
                        selectByMouse: true
                        wrapMode: TextEdit.WrapAnywhere
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
                        text: "Installation and updates reload Hyprland. Your Hyprland config is never edited."
                        textFormat: Text.PlainText
                        wrapMode: Text.Wrap
                    }
                    Flow {
                        Layout.fillWidth: true
                        spacing: 8
                        Button {
                            text: setup.info.phase === "install" ? "Install" : "Install / update"
                            visible: !!setup.info.path && !setup.info.available
                            enabled: !setup.info.busy
                            onClicked: setup.request("install")
                        }
                        Button {
                            text: "I connected it"
                            visible: !!setup.info.require && !setup.info.available
                            enabled: !setup.info.busy
                            onClicked: setup.request("confirm")
                        }
                        Button {
                            text: "Check again"
                            enabled: !setup.info.busy
                            onClicked: setup.request("refresh")
                        }
                        Button {
                            text: setup.info.available ? "Continue" : "Continue without integration"
                            onClicked: {
                                window.integrationStartupResolved = true
                                window.integrationOpen = false
                            }
                        }
                    }
                }
            }
        }
        Repeater {
            model: catalogState
            delegate: ColumnLayout {
                id: catalog
                visible: !window.integrationOpen
                Layout.fillWidth: true
                Layout.fillHeight: true
                readonly property bool loading: model.loading
                readonly property bool failed: model.failed
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
                            Component.onCompleted: {
                                launcher.query = window.integrationOpen ? integrationButton : query
                                if (window.visible) launcher.query.forceActiveFocus()
                            }
                            Connections {
                                target: window
                                function onIntegrationOpenChanged() {
                                    launcher.query = window.integrationOpen ? integrationButton : query
                                    if (window.visible) launcher.query.forceActiveFocus()
                                }
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
                            text: search.info.selected_id || ""
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
