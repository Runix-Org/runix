import QtQuick
import QtQuick.Controls

FocusScope {
    id: root
    property var state: null
    readonly property var info: state ? JSON.parse(state.snapshot) : ({loading: true, selected_index: -1})
    property int iconSize: 32
    property int delegateCount: 0
    readonly property alias view: list

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
    function activate() { if (info.activation_enabled) command("activate") }
    function inspect() {
        command("icons-stats")
        const rows = []
        for (const child of list.contentItem.children) {
            if (child.recordKey !== undefined && child.y + child.height > list.contentY
                    && child.y < list.contentY + list.height)
                rows.push({index: child.rowIndex, id: child.recordKey, title: child.rowTitle,
                    icon_status: child.iconStatus, icon_width: child.iconWidth})
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
        clip: true
        model: searchRows
        currentIndex: root.info.selected_index
        cacheBuffer: 0
        reuseItems: true
        boundsBehavior: Flickable.StopAtBounds
        ScrollBar.vertical: ScrollBar {}
        delegate: ItemDelegate {
            id: row
            required property int index
            required property string recordID
            required property string displayID
            required property string title
            required property string subtitle
            required property string category
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
            background: Rectangle {
                color: row.highlighted ? row.palette.highlight : "transparent"
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
                Column {
                    width: parent.width - root.iconSize - parent.spacing
                    anchors.verticalCenter: parent.verticalCenter
                    Label {
                        width: parent.width
                        text: row.title + (row.category ? "  ·  " + row.category : "")
                        textFormat: Text.PlainText
                        color: row.palette.text
                        elide: Text.ElideRight
                    }
                    Label {
                        width: parent.width
                        text: row.subtitle + "  [" + row.displayID + "]"
                        textFormat: Text.PlainText
                        font.pixelSize: 12
                        color: row.palette.text
                        elide: Text.ElideRight
                    }
                }
            }
            onClicked: root.select(index)
            onDoubleClicked: { root.select(index); root.activate() }
            Component.onCompleted: ++root.delegateCount
            Component.onDestruction: --root.delegateCount
        }
    }
    Label {
        anchors.centerIn: parent
        visible: root.info.loading || list.count === 0
        text: root.info.loading ? "Searching…" : "No results"
        padding: 12
        background: Rectangle { color: palette.window }
    }
}
