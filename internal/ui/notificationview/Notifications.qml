import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Item {
    id: root
    property var hostWindow
    property var searchControl
    property bool recording: false
    property var entries: []
    property int queued: 0
    property var expanded: null
    onExpandedChanged: {
        for (let i = 0; i < cards.count; ++i) cards.itemAt(i).reading()
    }
    property var returnFocus: null
    function request(op, id, value) {
        const target = bridge.itemAt(0)
        if (target) target.send(op, id || 0, !!value)
    }
    function containsFocus(item) {
        let focused = hostWindow ? hostWindow.activeFocusItem : null
        while (focused) {
            if (focused === item) return true
            focused = focused.parent
        }
        return false
    }
    onHostWindowChanged: { if (hostWindow) request("visible", 0, hostWindow.visible) }
    ListModel { id: cardModel }
    function restoreFocus() {
        const target = returnFocus && returnFocus.visible && returnFocus.enabled ? returnFocus : searchControl
        if (target) target.forceActiveFocus()
        returnFocus = null
    }
    function closeDetails() {
        if (!expanded) return false
        const id = expanded.id
        expanded = null
        detail.focus = false
        request("reading", id, false)
        Qt.callLater(restoreFocus)
        return true
    }
    function closeFocusedDetails() { return expanded && detail.activeFocus ? root.closeDetails() : false }
    function openDetails(entry, control) {
        returnFocus = control
        expanded = entry
        request("reading", entry.id, true)
        Qt.callLater(function() { fullText.forceActiveFocus() })
    }
    function focusNotifications() {
        if (expanded) { fullText.forceActiveFocus(); return }
        const first = cards.itemAt(0)
        if (first) first.focusDetails()
    }
    function fullTextFor(entry) {
        return entry ? entry.summary + (entry.details ? "\n\n" + entry.details : "")
            + (entry.count > 1 ? "\n\nRepeated " + entry.count + " times." : "") : ""
    }
    component Action: Loader {
        id: action
        property string text: ""
        signal clicked()
        sourceComponent: runixButtonComponent
        onLoaded: {
            item.text = Qt.binding(function() { return action.text })
            item.clicked.connect(action.clicked)
        }
    }
    Repeater {
        id: bridge
        model: notificationState
        delegate: Item {
            readonly property var info: JSON.parse(model.snapshot)
            function send(op, id, value) { model.command = JSON.stringify({op: op, id: id, value: value}) }
            onInfoChanged: {
                root.entries = info.entries
                root.queued = info.queued
                for (let i = cardModel.count - 1; i >= 0; --i) {
                    if (!info.entries.some(function(entry) { return entry.id === cardModel.get(i).entry.id }))
                        cardModel.remove(i)
                }
                for (let i = 0; i < info.entries.length; ++i) {
                    if (i < cardModel.count && cardModel.get(i).entry.id === info.entries[i].id)
                        cardModel.setProperty(i, "entry", info.entries[i])
                    else cardModel.insert(i, {entry: info.entries[i]})
                }
                if (root.expanded) {
                    const current = info.entries.find(function(entry) { return entry.id === root.expanded.id })
                    if (current) root.expanded = current
                    else root.closeDetails()
                }
            }
            Component.onCompleted: { if (root.hostWindow) send("visible", 0, root.hostWindow.visible) }
            Connections {
                target: root.hostWindow
                function onVisibleChanged() { send("visible", 0, root.hostWindow.visible) }
            }
        }
    }
    Shortcut {
        sequence: "Alt+N"
        enabled: !root.recording && root.entries.length > 0
        onActivated: root.focusNotifications()
    }
    Column {
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        anchors.margins: 12
        width: Math.min(440, root.width - 24)
        spacing: 6
        visible: !root.expanded
        Label {
            visible: root.entries.length > 0
            text: "Notifications · Alt+N"
            color: runixControlStyle.text
            padding: 4
            background: Rectangle { color: runixControlStyle.popup; radius: runixControlStyle.radius }
        }
        Repeater {
            id: cards
            model: cardModel
            delegate: Rectangle {
                id: card
                required property var entry
                readonly property var modelData: entry
                width: parent.width
                height: content.implicitHeight + 16
                radius: runixControlStyle.radius
                color: runixControlStyle.popup
                border.color: runixControlStyle.popupBorder
                readonly property string hue: ({error: "red", warning: "yellow", success: "green", information: "blue"})[modelData.severity] || "blue"
                function focusDetails() { if (details.item) details.item.forceActiveFocus() }
                function reading() {
                    root.request("reading", modelData.id, hover.hovered || root.containsFocus(card)
                        || !!root.expanded)
                }
                onActiveFocusChanged: reading()
                HoverHandler { id: hover; onHoveredChanged: card.reading() }
                Connections {
                    target: root.hostWindow
                    function onActiveFocusItemChanged() { card.reading() }
                }
                ColumnLayout {
                    id: content
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.top: parent.top
                    anchors.margins: 8
                    spacing: 4
                    RowLayout {
                        Layout.fillWidth: true
                        Label {
                            text: ({error: "Error", warning: "Warning", success: "Success", information: "Info"})[card.modelData.severity] || "Info"
                            color: runixControlStyle.hueIcon(card.hue)
                        }
                        Label {
                            Layout.fillWidth: true
                            text: card.modelData.summary
                            textFormat: Text.PlainText
                            elide: Text.ElideRight
                            color: runixControlStyle.text
                        }
                        Label {
                            visible: card.modelData.count > 1
                            text: "×" + card.modelData.count
                            color: runixControlStyle.secondary
                        }
                    }
                    RowLayout {
                        Action {
                            id: details
                            text: "Details"
                            onClicked: root.openDetails(card.modelData, item)
                        }
                        Action { text: "Copy"; onClicked: root.request("copy", card.modelData.id) }
                        Action {
                            text: "Dismiss"
                            onClicked: {
                                const focused = root.containsFocus(card)
                                root.request("dismiss", card.modelData.id)
                                if (focused && root.searchControl) root.searchControl.forceActiveFocus()
                            }
                        }
                        Item { Layout.fillWidth: true }
                    }
                }
            }
        }
        Label {
            visible: root.queued > 0
            text: root.queued + " more pending · Alt+N for notifications"
            color: runixControlStyle.text
            padding: 4
            background: Rectangle { color: runixControlStyle.popup; radius: runixControlStyle.radius }
        }
    }
    FocusScope {
        id: detail
        visible: !!root.expanded
        anchors.fill: parent
        anchors.margins: 12
        Rectangle {
            anchors.fill: parent
            color: runixControlStyle.popup
            border.color: runixControlStyle.popupBorder
            radius: runixControlStyle.radius
        }
        ColumnLayout {
            anchors.fill: parent
            anchors.margins: 12
            spacing: 8
            Label { text: "Notification details"; font.bold: true; color: runixControlStyle.text }
            Flickable {
                id: scroll
                Layout.fillWidth: true
                Layout.fillHeight: true
                clip: true
                boundsBehavior: Flickable.StopAtBounds
                ScrollBar.vertical: runixScrollBarComponent.createObject(null, {policy: ScrollBar.AsNeeded})
                TextArea.flickable: TextArea {
                    id: fullText
                    text: root.fullTextFor(root.expanded)
                    textFormat: TextEdit.PlainText
                    color: runixControlStyle.text
                    selectionColor: activeAppearance.surfaces.selection
                    selectedTextColor: runixControlStyle.selectedText
                    background: Item {}
                    readOnly: true
                    selectByMouse: true
                    persistentSelection: true
                    wrapMode: TextEdit.WrapAnywhere
                    activeFocusOnTab: true
                    KeyNavigation.tab: copyDetails.item
                    KeyNavigation.backtab: closeButton.item
                    onTextChanged: { cursorPosition = 0; scroll.contentY = 0 }
                }
            }
            RowLayout {
                Action {
                    id: copyDetails
                    text: "Copy full text"
                    onClicked: { if (root.expanded) root.request("copy", root.expanded.id) }
                    onLoaded: {
                        item.KeyNavigation.tab = dismissDetails.item
                        item.KeyNavigation.backtab = fullText
                    }
                }
                Action {
                    id: dismissDetails
                    text: "Dismiss"
                    onClicked: {
                        const id = root.expanded.id
                        root.closeDetails()
                        root.request("dismiss", id)
                    }
                    onLoaded: {
                        item.KeyNavigation.tab = closeButton.item
                        item.KeyNavigation.backtab = copyDetails.item
                    }
                }
                Item { Layout.fillWidth: true }
                Action {
                    id: closeButton
                    text: "Close"
                    onClicked: root.closeDetails()
                    onLoaded: {
                        item.KeyNavigation.tab = fullText
                        item.KeyNavigation.backtab = dismissDetails.item
                    }
                }
            }
        }
    }
}
