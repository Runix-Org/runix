import QtQuick
import QtQuick.Controls

FocusScope {
    id: root
    property var state: null
    readonly property var info: state ? JSON.parse(state.snapshot) : ({loading: true, selected_index: -1})
    property int iconSize: 32
    property int delegateCount: 0
    readonly property alias view: list
    property bool providerLoading: false
    property bool providerUnavailable: false

    function command(op, text, index) {
        if (state) state.command = JSON.stringify({op: op, text: text || "", index: index || 0})
    }
    function select(index) {
        if (info.loading || index < 0 || index >= list.count) return false
        list.cancelFlick()
        command("select", "", index)
        list.positionViewAtIndex(index, ListView.Contain)
        return true
    }
    function move(delta) {
        select(Math.max(0, Math.min(list.count - 1, info.selected_index + delta)))
    }
    function activationTarget() {
        return {id: info.selected_id || "", generation: info.applied_generation}
    }
    function activate(target) {
        if (!info.activation_enabled || info.loading || !state) return
        const captured = target || activationTarget()
        state.command = JSON.stringify({op: "activate", id: captured.id, generation: captured.generation})
    }
    function inspect() {
        command("icons-stats")
        const rows = []
        for (const child of list.contentItem.children) {
            if (child.recordKey !== undefined && child.y + child.height > list.contentY
                    && child.y < list.contentY + list.height)
                rows.push({index: child.rowIndex, id: child.recordKey, title: child.rowTitle,
                    icon_status: child.iconStatus, icon_width: child.iconWidth,
                    selected: child.highlighted, hovered: child.hovered, pressed: child.pressed})
        }
        rows.sort(function(a, b) { return a.index - b.index })
        return {state: info, delegates: delegateCount, viewport_height: list.height,
                content_y: list.contentY, visible_rows: rows}
    }
    onInfoChanged: {
        if (!info.loading && info.selected_index >= 0) {
            list.cancelFlick()
            list.positionViewAtIndex(info.selected_index, ListView.Contain)
        }
    }
    Keys.onUpPressed: move(-1)
    Keys.onDownPressed: move(1)
    Keys.onReturnPressed: function(event) { if (!event.isAutoRepeat) activate() }
    Keys.onEnterPressed: function(event) { if (!event.isAutoRepeat) activate() }

    ListView {
        id: list
        anchors.fill: parent
        anchors.bottomMargin: providerStatus.visible ? providerStatus.height : 0
        clip: true
        model: searchRows
        currentIndex: root.info.selected_index
        cacheBuffer: 0
        reuseItems: true
        boundsBehavior: Flickable.StopAtBounds
        ScrollBar.vertical: runixScrollBarComponent.createObject(null)
        delegate: ItemDelegate {
            id: row
            required property int index
            required property string recordID
            required property string displayID
            required property string title
            required property string subtitle
            required property string category
            required property string type
            required property string iconSource
            readonly property int iconStatus: rowImage.status
            readonly property int iconWidth: rowImage.implicitWidth
            readonly property string recordKey: recordID
            readonly property string rowTitle: title
            readonly property int rowIndex: index
            width: list.width
            height: Math.max(54, root.iconSize + 12)
            highlighted: index === root.info.selected_index
            enabled: !root.info.loading
            focusPolicy: Qt.NoFocus
            hoverEnabled: true
            background: Item {
                Rectangle {
                    anchors.fill: parent
                    color: row.highlighted ? activeAppearance.surfaces.selected
                        : runixControlStyle.wash(pointer)
                }
                Rectangle {
                    width: 2
                    height: parent.height
                    visible: row.highlighted
                    color: activeAppearance.foregrounds["selection-marker"]
                }
            }
            contentItem: Row {
                spacing: 10
                Image {
                    id: rowImage
                    anchors.verticalCenter: parent.verticalCenter
                    width: root.iconSize
                    height: root.iconSize
                    source: row.iconSource
                    cache: false
                    smooth: true
                    mipmap: true
                    fillMode: Image.PreserveAspectFit
                }
                Row {
                    width: parent.width - root.iconSize - parent.spacing
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: 8
                    Column {
                        width: parent.width - typeLabel.implicitWidth - parent.spacing
                        Label {
                            width: parent.width
                            text: row.title
                            textFormat: Text.PlainText
                            color: row.enabled ? runixControlStyle.text : runixControlStyle.disabled
                            elide: Text.ElideRight
                        }
                        Label {
                            width: parent.width
                            text: row.subtitle
                            textFormat: Text.PlainText
                            font.pixelSize: 12
                            color: row.enabled ? runixControlStyle.secondary : runixControlStyle.disabled
                            elide: Text.ElideRight
                        }
                    }
                    Label {
                        id: typeLabel
                        text: row.type || row.category
                        textFormat: Text.PlainText
                        font.pixelSize: 12
                        color: runixControlStyle.secondary
                        anchors.verticalCenter: parent.verticalCenter
                    }
                }
            }
            onClicked: root.activate({id: recordID, generation: root.info.applied_generation})
            MouseArea {
                id: pointer
                anchors.fill: parent
                acceptedButtons: Qt.LeftButton
                hoverEnabled: true
                property var target: null
                onPressed: target = {id: row.recordID, generation: root.info.applied_generation}
                onClicked: root.activate(target)
                // Accept the second press without sending a second activation.
                onDoubleClicked: function(event) { event.accepted = true }
            }
            Component.onCompleted: ++root.delegateCount
            Component.onDestruction: --root.delegateCount
        }
    }
    Label {
        anchors.centerIn: parent
        visible: list.count === 0
        text: root.info.loading ? "Searching…" : root.providerLoading ? "Loading items…"
            : root.providerUnavailable ? "Items are unavailable"
            : root.info.query ? "No query matches" : "No available items"
        textFormat: Text.PlainText
        color: runixControlStyle.text
        padding: 12
    }
    Label {
        id: providerStatus
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        visible: list.count > 0 && (root.providerLoading || root.providerUnavailable)
        text: root.providerLoading ? "Loading applications…" : "Some sources are unavailable"
        textFormat: Text.PlainText
        color: root.providerLoading ? runixControlStyle.loader : runixControlStyle.secondary
        padding: runixControlStyle.spacing
    }
}
