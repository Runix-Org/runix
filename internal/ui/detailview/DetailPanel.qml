import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    id: root
    property var state: null
    property string contentID: ""
    property var semanticDebug: null
    property var requestedID: null
    readonly property var info: state && state.snapshot
        ? JSON.parse(state.snapshot)
        : ({id: "", format: "", html: "", link_status: "", last_link: ""})
    readonly property color linkColor: activeAppearance.foregrounds["blue-text"]
    readonly property string renderedHTML: (info.html || "").replace(/<a href=/g,
        '<a style="color:' + linkColor + '; text-decoration:underline" href=')
    property string displayedSource: ""
    property int renderRevision: 0
    onRenderedHTMLChanged: {
        const replacing = displayedSource !== (info.html || "")
        const start = textArea.selectionStart
        const end = textArea.selectionEnd
        const cursor = textArea.cursorPosition
        const scrollY = scroller.contentY
        const revision = ++renderRevision
        // Qt ignores palette.link in rich text. Style only our sanitized anchors, never raw input.
        // Clear/insert also avoids stale rich text after replacing a scrolled document on Qt 6.12.
        textArea.clear()
        textArea.insert(0, renderedHTML)
        if (replacing) textArea.cursorPosition = 0
        else if (cursor === start) textArea.select(end, start)
        else textArea.select(start, end)
        displayedSource = info.html || ""
        Qt.callLater(function() {
            if (revision === root.renderRevision) scroller.contentY = replacing ? 0 : scrollY
        })
    }
    function requestContent() {
        if (!state || requestedID === contentID) return
        requestedID = contentID
        state.command = JSON.stringify({op: "content", id: contentID})
    }
    function copyContent() { textArea.copy() }
    function selectAllContent() { textArea.selectAll() }
    function inspectDetail() {
        return {id: info.id || "", format: info.format || "",
                selection_start: textArea.selectionStart,
                selection_end: textArea.selectionEnd,
                selected_length: Math.abs(textArea.selectionEnd - textArea.selectionStart),
                contentY: scroller.contentY,
                contentHeight: textArea.contentHeight, viewport: scroller.height,
                link_status: info.link_status || "", last_link: info.last_link || ""}
    }
    function scrollDetail(value) {
        if (value === "bottom") {
            scroller.contentY = Math.max(0, scroller.contentHeight - scroller.height)
            return true
        }
        const y = Number(value)
        if (!isFinite(y) || y < 0 || y > Math.max(0, scroller.contentHeight - scroller.height)) return false
        scroller.contentY = y
        return true
    }
    function registerDebug() {
        if (!semanticDebug) return
        semanticDebug.registerControl("detail", textArea, "text", "Detail content", ["focus", "select", "scroll"], null, false)
        semanticDebug.controls["detail"].inspect = inspectDetail
        semanticDebug.controls["detail"].readValue = function() { return textArea.getText(0, 256) }
        semanticDebug.controls["detail"].select = function(value) {
            if (value) return false
            selectAllContent()
            return true
        }
        semanticDebug.controls["detail"].scroll = scrollDetail
        semanticDebug.registerControl("detail-copy", copyButton.item, "button", "Copy detail", ["activate"], copyContent, false)
        semanticDebug.registerControl("detail-select-all", selectButton.item, "button", "Select all detail", ["activate"], selectAllContent, false)
    }
    onSemanticDebugChanged: registerDebug()
    onStateChanged: { requestedID = null; requestContent() }
    onContentIDChanged: {
        textArea.deselect()
        scroller.contentY = 0
        requestedID = null
        requestContent()
    }
    Component.onCompleted: { requestContent(); registerDebug() }

    RowLayout {
        Layout.fillWidth: true
        Loader {
            id: selectButton
            sourceComponent: runixButtonComponent
            onLoaded: { item.text = "Select all"; item.clicked.connect(root.selectAllContent) }
        }
        Loader {
            id: copyButton
            sourceComponent: runixButtonComponent
            onLoaded: { item.text = "Copy"; item.clicked.connect(root.copyContent) }
        }
        Label {
            Layout.fillWidth: true
            text: root.info.link_status || ""
            textFormat: Text.PlainText
            elide: Text.ElideRight
            color: runixControlStyle.secondary
        }
    }
    Flickable {
        id: scroller
        Layout.fillWidth: true
        Layout.fillHeight: true
        clip: true
        boundsBehavior: Flickable.StopAtBounds
        ScrollBar.vertical: runixScrollBarComponent.createObject(null, {policy: ScrollBar.AsNeeded})
        TextArea.flickable: TextArea {
            id: textArea
            color: runixControlStyle.text
            selectionColor: activeAppearance.surfaces.selection
            selectedTextColor: runixControlStyle.selectedText
            background: Item {}
            onTextChanged: {
                cursorPosition = 0
                deselect()
                scroller.contentY = 0
            }
            textFormat: TextEdit.RichText
            readOnly: true
            selectByMouse: true
            persistentSelection: true
            wrapMode: TextEdit.Wrap
            font.family: root.info.format === "code" ? "monospace" : Qt.application.font.family
            onLinkActivated: function(link) {
                if (root.state) root.state.command = JSON.stringify({op: "link", url: link})
            }
            Keys.onPressed: function(event) {
                if (event.key === Qt.Key_C && (event.modifiers & Qt.ControlModifier)) {
                    root.copyContent()
                    event.accepted = true
                }
            }
        }
    }
}
