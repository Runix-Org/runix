import QtQuick
import QtQuick.Controls

Item {
    id: root
    property string kind: "keycap"
    property string text: ""
    property string detail: ""
    property string hue: "blue"
    property bool recording: false
    property string shortcut: "Ctrl+K"
    readonly property bool conflict: sequence === "Ctrl+K"
    property string sequence: ""
    implicitWidth: kind === "recorder" ? 260 : kind === "keycap" ? 64 : 180
    implicitHeight: content.implicitHeight + 12
    activeFocusOnTab: kind === "recorder"
    onActiveFocusChanged: { if (!activeFocus) recording = false }
    onVisibleChanged: { if (!visible) recording = false }
    Keys.onShortcutOverride: event => { if (root.recording) event.accepted = true }
    Keys.onPressed: event => {
        if (root.kind !== "recorder" || !root.enabled) return
        if (!root.recording) {
            if (event.key === Qt.Key_Space || event.key === Qt.Key_Return) {
                root.recording = true
                root.sequence = ""
                event.accepted = true
            }
            return
        }
        if ([Qt.Key_Control, Qt.Key_Shift, Qt.Key_Alt, Qt.Key_Meta].indexOf(event.key) >= 0) {
            event.accepted = true
            return
        }
        if (event.key === Qt.Key_Escape) {
            root.sequence = ""
            root.recording = false
            event.accepted = true
            return
        }
        const parts = []
        if (event.modifiers & Qt.ControlModifier) parts.push("Ctrl")
        if (event.modifiers & Qt.AltModifier) parts.push("Alt")
        if (event.modifiers & Qt.ShiftModifier) parts.push("Shift")
        if (event.modifiers & Qt.MetaModifier) parts.push("Meta")
        const namedKeys = [Qt.Key_Space, Qt.Key_Return, Qt.Key_Tab, Qt.Key_Backspace,
            Qt.Key_Delete, Qt.Key_Left, Qt.Key_Right, Qt.Key_Up, Qt.Key_Down]
        const names = ["Space", "Enter", "Tab", "Backspace", "Delete", "Left", "Right", "Up", "Down"]
        const namedIndex = namedKeys.indexOf(event.key)
        const name = namedIndex >= 0 ? names[namedIndex] : event.key >= 33 && event.key <= 126
            ? String.fromCharCode(event.key) : "Key " + event.key
        if (name) parts.push(name)
        if (parts.length) {
            root.sequence = parts.join("+")
            root.recording = false
        }
        event.accepted = true
    }
    Rectangle {
        anchors.fill: parent
        radius: runixControlStyle.radius
        color: root.kind === "tag" ? runixControlStyle.tagFill(root.hue)
            : root.kind === "keycap" || root.kind === "recorder" ? runixControlStyle.button : "transparent"
        border.width: root.kind === "keycap" || root.kind === "recorder" ? 1 : 0
        border.color: runixControlStyle.separator
    }
    Column {
        id: content
        anchors.centerIn: parent
        width: Math.max(0, root.width - 16)
        spacing: 3
        Row {
            spacing: 6
            Text {
                textFormat: Text.PlainText
                text: root.kind === "glyph" ? root.text : ({red: "!", yellow: "⚠", green: "✓", blue: "i"})[root.hue] || "i"
                visible: root.kind === "glyph" || root.kind === "status"
                font.pixelSize: 20
                color: root.kind === "glyph" ? runixControlStyle.icon : runixControlStyle.hueIcon(root.hue)
            }
            Text {
                textFormat: Text.PlainText
                text: root.kind === "recorder"
                    ? (root.recording ? "Press a key…" : root.sequence || root.shortcut)
                    : root.text
                font.bold: ["keycap", "tag", "section", "recorder"].indexOf(root.kind) >= 0
                color: root.kind === "section" ? runixControlStyle.secondary
                    : root.kind === "tag" ? (root.enabled ? runixControlStyle.hueText(root.hue) : runixControlStyle.disabled)
                        : runixControlStyle.text
                visible: root.kind !== "glyph"
            }
            Text {
                text: "◌"
                visible: root.kind === "loading"
                color: runixControlStyle.loader
                RotationAnimator on rotation {
                    from: 0; to: 360; duration: 900; loops: Animation.Infinite
                    running: root.kind === "loading" && root.visible
                }
            }
        }
        Rectangle {
            width: content.width
            height: 1
            color: runixControlStyle.separator
            visible: root.kind === "section"
        }
        Text {
            width: parent.width
            textFormat: Text.PlainText
            text: root.kind === "recorder" ? (root.recording ? "Escape cancels. No global shortcut is changed."
                : root.conflict ? "! Ctrl+K is used by the sample action."
                : "Press Enter or click to record. Ctrl+K shows a sample conflict.") : root.detail
            visible: text.length > 0
            color: root.conflict && root.kind === "recorder" ? runixControlStyle.error
                : root.kind === "status" ? runixControlStyle.text : runixControlStyle.secondary
            wrapMode: Text.Wrap
        }
    }
    Rectangle {
        anchors.fill: parent
        anchors.margins: -2
        radius: runixControlStyle.radius + 2
        color: "transparent"
        border.width: root.activeFocus ? 2 : 0
        border.color: runixControlStyle.focus
        visible: root.kind === "recorder"
    }
    MouseArea {
        anchors.fill: parent
        enabled: root.kind === "recorder" && root.enabled
        onClicked: {
            root.forceActiveFocus()
            root.recording = true
            root.sequence = ""
        }
    }
}
