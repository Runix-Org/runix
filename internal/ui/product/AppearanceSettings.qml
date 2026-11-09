import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ScrollView {
    id: root
    property var appearance: activeAppearance
    component ThemePreview: Pane {
        id: themePreview
        required property var theme
        Layout.fillWidth: true
        padding: 10
        background: Item {}
        ColumnLayout {
            anchors.fill: parent
            Rectangle {
                Layout.fillWidth: true
                height: 48
                gradient: Gradient {
                    GradientStop { position: 0; color: themePreview.theme.background || "transparent" }
                    GradientStop { position: 0.7; color: themePreview.theme.background_secondary || "transparent" }
                    GradientStop { position: 1; color: themePreview.theme.background_secondary || "transparent" }
                }
                Label {
                    anchors.centerIn: parent
                    text: "Sample text"
                    color: themePreview.theme.text || runixControlStyle.text
                }
            }
            Label {
                Layout.fillWidth: true
                text: (themePreview.theme.name || "Unnamed theme") + " · "
                    + (themePreview.theme.author || themePreview.theme.author_username || "No author provided")
                    + " · " + (themePreview.theme.id || "Unavailable")
                    + " · " + (themePreview.theme.appearance || "")
                wrapMode: Text.Wrap; textFormat: Text.PlainText; color: runixControlStyle.secondary
            }
        }
    }
    ScrollBar.vertical: runixScrollBarComponent.createObject(null)
    background: Item {}
    clip: true
    contentWidth: availableWidth
    contentHeight: content.implicitHeight + 8

    function label(theme) {
        return theme ? (theme.name || "Unnamed theme") + " — "
            + (theme.author || theme.author_username || "No author provided") + " — " + theme.id : ""
    }
    function save(prefs) {
        status.text = appearance.setPreferences(prefs) ? ""
            : "Could not save appearance settings. Your previous choice is still active."
    }
    function selected(themes, id) {
        for (let i = 0; i < themes.length; ++i) if (themes[i].id === id) return i
        return -1
    }
    function selectedTheme(themes, id) {
        const index = selected(themes, id)
        return index >= 0 ? themes[index] : null
    }
    function colorLabel(c) {
        return "rgb(" + Math.round(c.r * 255) + ", " + Math.round(c.g * 255)
            + ", " + Math.round(c.b * 255) + ")"
    }
    function prepare(control, dropdown) {
        control.activeFocusChanged.connect(function() {
            if (!control.activeFocus) return
            Qt.callLater(function() {
                const view = root.contentItem
                const point = control.mapToItem(view.contentItem, 0, 0)
                let y = view.contentY
                if (point.y - 4 < y) y = point.y - 4
                else if (point.y + control.height + 4 > y + view.height)
                    y = point.y + control.height + 4 - view.height
                view.contentY = Math.max(0, Math.min(y, Math.max(0, view.contentHeight - view.height)))
            })
        })
        if (dropdown) control.popup.height = Qt.binding(function() {
            return Math.min(240, Math.max(100, root.height - 24), control.popup.contentItem.implicitHeight + 8)
        })
    }
    Component.onCompleted: {
        const order = [modeBox.item, lightBox.item, darkBox.item, improveBox.item,
            jsonInput.item, pasteButton.item, filePath.item, fileButton.item]
        for (let i = 0; i < order.length; ++i) {
            const control = order[i]
            prepare(control, i < 3)
            control.KeyNavigation.priority = KeyNavigation.BeforeItem
            control.KeyNavigation.tab = order[(i + 1) % order.length]
            control.KeyNavigation.backtab = order[(i + order.length - 1) % order.length]
        }
    }

    ColumnLayout {
        id: content
        x: 4; y: 4
        width: Math.max(0, root.availableWidth - 8)
        spacing: 12
        Label { text: "Appearance"; font.pixelSize: 18; font.bold: true }
        Label {
            text: "Choose how runix looks. Light and dark themes are saved separately."
            color: runixControlStyle.secondary
            wrapMode: Text.Wrap
            Layout.fillWidth: true
        }
        Label { text: "Mode"; color: runixControlStyle.secondary }
        Loader {
            id: modeBox
            Layout.fillWidth: true
            sourceComponent: runixComboBoxComponent
            onLoaded: {
                item.model = ["System", "Light", "Dark"]
                item.currentIndex = ({system: 0, light: 1, dark: 2})[root.appearance.preferences.mode || "system"] ?? 0
                item.activated.connect(function(index) {
                    const p = Object.assign({}, root.appearance.preferences, {mode: ["system", "light", "dark"][index]})
                    root.save(p)
                    item.currentIndex = ({system: 0, light: 1, dark: 2})[
                        root.appearance.preferences.mode || "system"] ?? 0
                })
            }
        }
        Label { text: "Light theme"; color: runixControlStyle.secondary }
        Loader {
            id: lightBox; Layout.fillWidth: true; sourceComponent: runixComboBoxComponent
            onLoaded: {
                refresh()
                item.activated.connect(function(index) {
                    const themes = root.appearance.lightThemes
                    const p = Object.assign({}, root.appearance.preferences, {light_id: themes[index].id})
                    root.save(p); refresh()
                })
            }
            function refresh() {
                if (!item) return
                item.model = root.appearance.lightThemes.map(function(theme) { return root.label(theme) })
                item.currentIndex = root.selected(root.appearance.lightThemes, root.appearance.preferences.light_id)
            }
            Connections {
                target: root.appearance
                function onRevisionChanged() { lightBox.refresh(); darkBox.refresh() }
            }
        }
        Label { text: "Dark theme"; color: runixControlStyle.secondary }
        Loader {
            id: darkBox; Layout.fillWidth: true; sourceComponent: runixComboBoxComponent
            onLoaded: {
                refresh()
                item.activated.connect(function(index) {
                    const themes = root.appearance.darkThemes
                    const p = Object.assign({}, root.appearance.preferences, {dark_id: themes[index].id})
                    root.save(p); refresh()
                })
            }
            function refresh() {
                if (!item) return
                item.model = root.appearance.darkThemes.map(function(theme) { return root.label(theme) })
                item.currentIndex = root.selected(root.appearance.darkThemes, root.appearance.preferences.dark_id)
            }
        }
        ThemePreview {
            theme: root.selectedTheme(root.appearance.lightThemes, root.appearance.preferences.light_id)
                || ({id: "", name: "No saved light theme"})
        }
        ThemePreview {
            theme: root.selectedTheme(root.appearance.darkThemes, root.appearance.preferences.dark_id)
                || ({id: "", name: "No saved dark theme"})
        }
        Label {
            Layout.fillWidth: true
            text: "Saved light ID: " + (root.appearance.preferences.light_id || "not set")
            color: runixControlStyle.secondary; wrapMode: Text.Wrap; textFormat: Text.PlainText
        }
        Label {
            Layout.fillWidth: true
            text: "Saved dark ID: " + (root.appearance.preferences.dark_id || "not set")
            color: runixControlStyle.secondary; wrapMode: Text.Wrap; textFormat: Text.PlainText
        }
        Label {
            Layout.fillWidth: true
            text: "Active theme ID: " + (root.appearance.activeID || "unknown")
            color: runixControlStyle.secondary; wrapMode: Text.Wrap; textFormat: Text.PlainText
        }
        Loader {
            id: improveBox; sourceComponent: runixCheckBoxComponent
            property bool restoring: false
            onLoaded: {
                item.text = "Improve contrast"
                item.checked = !!root.appearance.preferences.improve_contrast
                item.checkedChanged.connect(function() {
                    if (improveBox.restoring) return
                    const p = Object.assign({}, root.appearance.preferences, {improve_contrast: item.checked})
                    root.save(p)
                    improveBox.restoring = true
                    item.checked = !!root.appearance.preferences.improve_contrast
                    improveBox.restoring = false
                })
            }
        }
        Rectangle {
            Layout.fillWidth: true
            height: 88
            gradient: Gradient {
                GradientStop { position: 0; color: root.appearance.surfaces.b0 }
                GradientStop { position: 0.7; color: root.appearance.surfaces.b1 }
                GradientStop { position: 1; color: root.appearance.surfaces.b1 }
            }
            RowLayout {
                anchors.fill: parent
                anchors.margins: 8
                ColumnLayout {
                    Layout.fillWidth: true
                    Label { text: "Normal text"; color: root.appearance.originalForegrounds.primary }
                    Label { text: "Supporting text"; color: root.appearance.originalForegrounds.secondary }
                }
                ColumnLayout {
                    Layout.fillWidth: true
                    Label { text: "Effective text"; color: root.appearance.foregrounds.primary }
                    Label { text: "Supporting text"; color: root.appearance.foregrounds.secondary }
                }
            }
        }
        Label {
            Layout.fillWidth: true
            text: "Contrast checks use sampled backgrounds. Text color can change, but theme backgrounds "
                + "and gradients do not. Some gradients cannot meet every contrast target."
            color: runixControlStyle.secondary; wrapMode: Text.Wrap
        }
        Label {
            id: status; Layout.fillWidth: true; visible: text.length > 0
            color: runixControlStyle.error; textFormat: Text.PlainText; wrapMode: Text.Wrap
        }
        Label { text: "Import a theme"; font.bold: true }
        Label {
            Layout.fillWidth: true
            text: "Paste original Theme Studio JSON, or enter a local absolute file path. Import does not select "
                + "the theme. After import, choose it in the matching light or dark slot."
            color: runixControlStyle.secondary
            wrapMode: Text.Wrap
        }
        Label { text: "Original JSON"; color: runixControlStyle.secondary }
        Loader {
            id: jsonInput
            Layout.fillWidth: true
            Layout.preferredHeight: 110
            sourceComponent: runixTextAreaComponent
            onLoaded: item.placeholderText = "Paste original theme JSON"
        }
        RowLayout {
            Layout.fillWidth: true
            Loader {
                id: pasteButton; sourceComponent: runixButtonComponent
                onLoaded: {
                    item.text = "Import JSON"
                    item.clicked.connect(function() {
                        root.importResult(root.appearance.importJSON(jsonInput.item.text))
                    })
                }
            }
        }
        Label { text: "Local JSON file path"; color: runixControlStyle.secondary }
        Loader {
            id: filePath
            Layout.fillWidth: true
            sourceComponent: runixTextFieldComponent
            onLoaded: item.placeholderText = "Local absolute file path"
        }
        Loader {
            id: fileButton; sourceComponent: runixButtonComponent
            onLoaded: {
                item.text = "Import file"
                item.clicked.connect(function() { root.appearance.importFile(filePath.item.text) })
            }
        }
        Label {
            Layout.fillWidth: true
            text: root.appearance.importStatus || ""
            visible: text.length > 0; textFormat: Text.PlainText; wrapMode: Text.Wrap
            color: runixControlStyle.text
        }
        Label { text: "Contrast diagnostics"; font.bold: true }
        Repeater {
            model: root.appearance.diagnostics || []
            delegate: Label {
                required property var modelData
                Layout.fillWidth: true
                readonly property var diagnostic: modelData.diagnostic
                text: modelData.name + ": " + Number(diagnostic.minimum).toFixed(2)
                    + ":1 / target " + Number(diagnostic.target).toFixed(1)
                    + ":1 · " + (diagnostic.target === 0 ? "Not rated" : diagnostic.passing ? "Pass" : "Below target")
                    + " · " + (diagnostic.changed ? "Adjusted" : "Original")
                    + "\nWorst sample: " + diagnostic.worst.context + " on " + root.colorLabel(diagnostic.worst.color)
                    + " · y=" + Number(diagnostic.worst.y).toFixed(3)
                textFormat: Text.PlainText
                wrapMode: Text.Wrap
                color: runixControlStyle.text
            }
        }
    }
    function importResult(ok) {
        if (ok && jsonInput.item) jsonInput.item.text = ""
    }
}
