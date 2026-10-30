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
    readonly property string renderedHTML: info.html || ""
    readonly property var theme: themeLoader.item
    Loader { id: themeLoader; sourceComponent: labThemeComponent }
    onRenderedHTMLChanged: {
        // Editing operations avoid stale rich-text rendering after long-document replacement on Qt 6.12.
        textArea.clear()
        textArea.insert(0, renderedHTML)
        textArea.cursorPosition = 0
        Qt.callLater(function() { scroller.contentY = 0 })
    }

    function requestContent() {
        if (!state || requestedID === contentID)
            return
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
        if (!isFinite(y) || y < 0 || y > Math.max(0, scroller.contentHeight - scroller.height))
            return false
        scroller.contentY = y
        return true
    }
    function registerDebug() {
        if (!semanticDebug) return
        semanticDebug.registerControl("detail", textArea, "text", "Detail content",
                                      ["focus", "select", "scroll"], null, false)
        semanticDebug.controls["detail"].inspect = inspectDetail
        semanticDebug.controls["detail"].readValue = function() { return textArea.getText(0, 256) }
        semanticDebug.controls["detail"].select = function(value) {
            if (value) return false
            selectAllContent()
            return true
        }
        semanticDebug.controls["detail"].scroll = scrollDetail
        semanticDebug.registerControl("detail-copy", copyButton, "button", "Copy detail",
                                      ["activate"], copyContent, false)
        semanticDebug.registerControl("detail-select-all", selectButton, "button", "Select all detail",
                                      ["activate"], selectAllContent, false)
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
        Button { id: selectButton; text: "Select all"; onClicked: root.selectAllContent() }
        Button { id: copyButton; text: "Copy"; onClicked: root.copyContent() }
        Label {
            Layout.fillWidth: true
            text: root.info.link_status || ""
            textFormat: Text.PlainText
            elide: Text.ElideRight
        }
    }
    Flickable {
        id: scroller
        Layout.fillWidth: true
        Layout.fillHeight: true
        clip: true
        boundsBehavior: Flickable.StopAtBounds
        ScrollBar.vertical: ScrollBar {}
        TextArea.flickable: TextArea {
            id: textArea
            color: root.theme.foreground
            selectionColor: root.theme.accent
            selectedTextColor: root.theme.field
            palette.link: root.theme.accent
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
                if (root.state)
                    root.state.command = JSON.stringify({op: "link", url: link})
            }
            Keys.onPressed: function(event) {
                if ((event.key === Qt.Key_C) && (event.modifiers & Qt.ControlModifier)) {
                    root.copyContent()
                    event.accepted = true
                }
            }
        }
    }
}
