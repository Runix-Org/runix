import QtQuick
import QtQuick.Controls

FocusScope {
    id: root
    property var state: null
    readonly property var info: state ? JSON.parse(state.snapshot) : ({count: 0, selected_index: -1, sort_column: 0, descending: false})
    property int delegateCount: 0
    readonly property alias view: table

    readonly property int hoveredRow: rowHover.hovered
        ? Math.min(table.rows - 1, Math.floor((rowHover.point.position.y + table.contentY) / 36)) : -1
    property int pressedRow: -1
    HoverHandler { id: rowHover; parent: table }
    // Paint each row once across the viewport, including space beyond the last column.
    Item {
        anchors.fill: table
        clip: true
        Rectangle {
            width: parent.width
            height: 36
            y: root.info.selected_index * 36 - table.contentY
            visible: root.info.selected_index >= 0
            color: runixControlStyle.selectedRow
            Rectangle { width: 2; height: parent.height; color: runixControlStyle.selectionMarker }
        }
        Rectangle {
            width: parent.width
            height: 36
            y: (root.pressedRow >= 0 ? root.pressedRow : root.hoveredRow) * 36 - table.contentY
            visible: (root.pressedRow >= 0 ? root.pressedRow : root.hoveredRow) >= 0
                && (root.pressedRow >= 0 ? root.pressedRow : root.hoveredRow) !== root.info.selected_index
            color: root.pressedRow >= 0 ? runixControlStyle.press : runixControlStyle.hover
        }
    }
    function command(op, index, column, id) {
        if (state) state.command = JSON.stringify({op: op, index: index || 0, column: column || 0, id: id || ""})
    }
    function select(index) {
        if (index < 0 || index >= table.rows) return false
        command("select", index)
        table.positionViewAtRow(index, TableView.Contain)
        table.forceActiveFocus()
        return true
    }
    function activate(id) {
        const recordID = id || root.info.selected_id
        if (recordID) command("activate", 0, 0, recordID)
    }
    function inspect() {
        const cells = []
        for (const child of table.contentItem.children) {
            if (child.visible && child.recordID !== undefined && child.x + child.width > table.contentX
                    && table.itemAtCell(Qt.point(child.column, child.row)) === child
                    && child.x < table.contentX + table.width
                    && child.y + child.height > table.contentY && child.y < table.contentY + table.height)
                cells.push({row: child.row, column: child.column, id: child.recordID, text: child.cellText})
        }
        cells.sort(function(a, b) { return a.row === b.row ? a.column - b.column : a.row - b.row })
        return {state: info, delegates: delegateCount, column_widths: [table.columnWidth(0), table.columnWidth(1),
                table.columnWidth(2), table.columnWidth(3)], content_x: table.contentX, content_y: table.contentY,
                content_width: table.contentWidth, content_height: table.contentHeight, viewport_width: table.width,
                viewport_height: table.height, visible_cells: cells}
    }
    function sort(column) { command("sort", 0, column) }
    onInfoChanged: Qt.callLater(function() {
        table.forceLayout()
        if (info.selected_index >= 0 && info.selected_index < table.rows)
            table.positionViewAtRow(info.selected_index, TableView.Contain)
    })
    Keys.onUpPressed: select(info.selected_index - 1)
    Keys.onDownPressed: select(info.selected_index + 1)
    Keys.onPressed: function(event) {
        const page = Math.max(1, Math.floor(table.height / 36))
        switch (event.key) {
        case Qt.Key_PageUp: select(Math.max(0, info.selected_index - page)); break
        case Qt.Key_PageDown: select(Math.min(table.rows - 1, info.selected_index + page)); break
        case Qt.Key_Home: select(0); break
        case Qt.Key_End: select(table.rows - 1); break
        default: return
        }
        event.accepted = true
    }
    Keys.onReturnPressed: activate()
    Keys.onEnterPressed: activate()

    HorizontalHeaderView {
        id: header
        anchors.left: table.left
        anchors.right: table.right
        anchors.top: parent.top
        syncView: table
        clip: true
        resizableColumns: false
        delegate: Button {
            id: headerButton
            required property var model
            required property int index
            implicitHeight: 36
            implicitWidth: 100
            text: model.display
            onClicked: root.sort(index)
            background: Item {
                Rectangle { anchors.fill: parent; color: runixControlStyle.wash(headerButton) }
                Rectangle {
                    anchors.fill: parent
                    anchors.margins: 2
                    color: "transparent"
                    border.width: headerButton.visualFocus ? 2 : 0
                    border.color: runixControlStyle.focus
                    radius: runixControlStyle.radius
                }
                Rectangle { anchors.left: parent.left; anchors.right: parent.right; anchors.bottom: parent.bottom; height: 1; color: runixControlStyle.separator }
            }
            contentItem: Item {
                Text {
                    anchors.left: parent.left
                    anchors.right: sortGlyph.left
                    anchors.verticalCenter: parent.verticalCenter
                    text: headerButton.text
                    color: root.info.sort_column === headerButton.index
                        ? runixControlStyle.text : runixControlStyle.secondary
                    font: headerButton.font
                    elide: Text.ElideRight
                }
                Text {
                    id: sortGlyph
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    width: 18
                    text: root.info.sort_column === headerButton.index ? (root.info.descending ? "▼" : "▲") : ""
                    color: runixControlStyle.selectionMarker
                }
            }
            MouseArea {
                anchors.right: parent.right
                width: 8
                height: parent.height
                cursorShape: Qt.SizeHorCursor
                preventStealing: true
                property real startX: 0
                property real startWidth: 0
                onPressed: function(mouse) {
                    startX = mapToItem(root, mouse.x, mouse.y).x
                    startWidth = table.columnWidth(headerButton.index)
                }
                onPositionChanged: function(mouse) {
                    if (!pressed) return
                    const delta = mapToItem(root, mouse.x, mouse.y).x - startX
                    table.setColumnWidth(headerButton.index, Math.max(60, startWidth + delta))
                }
            }
            Component.onCompleted: {
                if (typeof semanticDebug !== "undefined")
                    semanticDebug.registerControl("table-sort-" + index, headerButton, "button", "Sort " + model.display,
                        ["activate"], function() { headerButton.clicked() }, false)
            }
            Component.onDestruction: {
                if (typeof semanticDebug !== "undefined"
                        && semanticDebug.controls["table-sort-" + index]?.item === headerButton)
                    delete semanticDebug.controls["table-sort-" + index]
            }
        }
    }
    TableView {
        id: table
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: header.bottom
        anchors.bottom: parent.bottom
        clip: true
        focus: true
        activeFocusOnTab: true
        keyNavigationEnabled: false
        model: tableRows
        rowSpacing: 0
        columnSpacing: 0
        reuseItems: true
        boundsBehavior: Flickable.StopAtBounds
        onMovementStarted: root.pressedRow = -1
        ScrollBar.vertical: runixScrollBarComponent.createObject(null, {policy: ScrollBar.AsNeeded})
        ScrollBar.horizontal: runixScrollBarComponent.createObject(null, {policy: ScrollBar.AsNeeded})
        columnWidthProvider: function(column) {
            const explicit = table.explicitColumnWidth(column)
            return explicit >= 0 ? explicit : [260, 100, 140, 220][column]
        }
        rowHeightProvider: function(row) { return 36 }
        delegate: Item {
            id: cell
            required property int row
            required property int column
            required property string display
            required property string recordID
            readonly property string cellText: display
            readonly property string recordKey: recordID
            readonly property bool hovered: root.hoveredRow === row
            readonly property bool pressed: root.pressedRow === row
            width: table.columnWidth(column)
            height: 36
            Text {
                anchors.fill: parent
                anchors.leftMargin: 8
                anchors.rightMargin: 8
                verticalAlignment: Text.AlignVCenter
                text: cell.cellText
                textFormat: Text.PlainText
                color: runixControlStyle.text
                elide: Text.ElideRight
            }
            MouseArea {
                anchors.fill: parent
                onPressed: root.pressedRow = cell.row
                onReleased: root.pressedRow = -1
                onCanceled: root.pressedRow = -1
                onClicked: { root.select(cell.row); table.forceActiveFocus() }
                onDoubleClicked: root.activate(cell.recordID)
            }
            Component.onCompleted: ++root.delegateCount
            Component.onDestruction: --root.delegateCount
        }
    }
    Label {
        anchors.centerIn: table
        visible: table.rows === 0
        text: "No records"
        color: runixControlStyle.secondary
    }
}
