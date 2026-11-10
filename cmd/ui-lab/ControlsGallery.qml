import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ScrollView {
    id: gallery
    ScrollBar.vertical: runixScrollBarComponent.createObject(null)
    clip: true
    contentWidth: availableWidth
    contentHeight: galleryContent.implicitHeight + 8
    readonly property var firstInput: searchField.item
    readonly property var controlPicker: controlPickerLoader.item
    readonly property var rolePicker: rolePickerLoader.item
    property var controls: ({})
    property string selectedID: "gallery-search"
    property string selectedRole: "primary"
    readonly property var selectedControl: controls[selectedID]
    property int activationCount: 0
    function clearPassword() {
        if (passwordField.item) passwordField.item.clear()
    }
    onVisibleChanged: if (!visible) clearPassword()
    function reveal(item) {
        const view = contentItem
        const y = item.mapToItem(galleryContent, 0, 0).y + galleryContent.y
        const maximum = Math.max(0, contentHeight - availableHeight)
        if (y < view.contentY) view.contentY = Math.max(0, y - 4)
        else if (y + item.height > view.contentY + availableHeight)
            view.contentY = Math.min(maximum, y + item.height + 4 - availableHeight)
    }
    function inspect(control) {
        const result = {
            focused: control.activeFocus, visual_focus: control.visualFocus,
            hovered: control.hovered, pressed: control.pressed,
            opened: control.opened === undefined ? null : control.opened,
            popup_open: control.popup && control.popup.visible !== undefined ? control.popup.visible
                : control === actionMenuButton.item ? actionMenu.item.opened
                : control === dialogButton.item ? dialogPopup.item.opened
                : control === popoverButton.item ? popoverPopup.item.opened
                : control === tooltipButton.item ? tooltipPopup.item.visible : null,
            checked: control.checked === undefined ? null : control.checked,
            read_only: control.readOnly === undefined ? null : control.readOnly,
            value: control.value === undefined ? null : control.value,
            selection_start: control.selectionStart === undefined ? null : control.selectionStart,
            selection_end: control.selectionEnd === undefined ? null : control.selectionEnd,
            busy: control.busy === undefined ? null : control.busy,
            enabled: control.enabled, activations: activationCount
        }
        if (control.currentIndex !== undefined) {
            result.current_index = control.currentIndex
            result.display_text = control.displayText
        }
        if (control.recording !== undefined) {
            result.recording = control.recording
            result.shortcut = control.sequence
        }
        if (control === actionMenuButton.item) result.menu_index = actionMenu.item.currentIndex
        return result
    }
    ButtonGroup { id: radioChoices }
    ColumnLayout {
        id: galleryContent
        x: 4
        y: 4
        width: Math.max(0, gallery.availableWidth - 8)
        spacing: runixControlStyle.spacing
        Label { text: "Shared controls"; font.pixelSize: 18 }
        Label {
            Layout.fillWidth: true
            wrapMode: Text.Wrap
            color: activeAppearance.foregrounds.secondary
            text: "Use Tab, Space, arrow keys, and the pointer. These specimens use the shared product controls."
        }
        Label { text: "Search" }
        Loader {
            id: searchField
            Layout.fillWidth: true
            sourceComponent: runixTextFieldComponent
            onLoaded: { item.search = true; item.placeholderText = "Search text" }
        }
        Label { text: "Text" }
        Loader {
            id: plainField
            Layout.fillWidth: true
            sourceComponent: runixTextFieldComponent
            onLoaded: { item.placeholderText = "Enter text"; item.text = "Editable text" }
        }
        Label { text: "Password" }
        Loader {
            id: passwordField
            Layout.fillWidth: true
            sourceComponent: runixTextFieldComponent
            onLoaded: { item.placeholderText = "Enter a sample password"; item.echoMode = TextInput.Password }
        }
        Label { text: "Validation and editing focus" }
        Loader {
            id: errorField
            Layout.fillWidth: true
            sourceComponent: runixTextFieldComponent
            onLoaded: { item.errorText = "This value needs attention"; item.text = "Invalid value" }
        }
        Label {
            Layout.fillWidth: true
            wrapMode: Text.Wrap
            text: "! This value needs attention"
            color: activeAppearance.foregrounds["red-text"]
        }
        Label { text: "Read-only text — select and copy" }
        Loader {
            id: readOnlyField
            Layout.fillWidth: true
            sourceComponent: runixTextFieldComponent
            onLoaded: { item.readOnly = true; item.text = "Select and copy this text" }
        }
        Label { text: "Disabled text" }
        Loader {
            id: disabledField
            Layout.fillWidth: true
            sourceComponent: runixTextFieldComponent
            onLoaded: { item.enabled = false; item.text = "Disabled text" }
        }
        Label { text: "Multiline notes" }
        Loader {
            id: notes
            Layout.fillWidth: true
            Layout.preferredHeight: 100
            sourceComponent: runixTextAreaComponent
            onLoaded: { item.placeholderText = "Enter notes"; item.text = "First line\nSecond line" }
        }
        Label { text: "Multiline validation" }
        Loader {
            id: errorNotes
            Layout.fillWidth: true
            Layout.preferredHeight: 70
            sourceComponent: runixTextAreaComponent
            onLoaded: { item.errorText = "Review these notes"; item.text = "Example notes" }
        }
        Label { text: "! Review these notes"; color: activeAppearance.foregrounds["red-text"] }
        Label { text: "Read-only notes" }
        Loader {
            id: readOnlyNotes
            Layout.fillWidth: true
            Layout.preferredHeight: 70
            sourceComponent: runixTextAreaComponent
            onLoaded: { item.readOnly = true; item.text = "Read-only notes\nSelect and copy either line" }
        }
        Label { text: "Disabled notes" }
        Loader {
            id: disabledNotes
            Layout.fillWidth: true
            Layout.preferredHeight: 70
            sourceComponent: runixTextAreaComponent
            onLoaded: { item.enabled = false; item.text = "Disabled notes" }
        }
        Label { text: "Buttons"; font.bold: true }
        GridLayout {
            columns: 4
            Loader {
                id: actionButton
                sourceComponent: runixButtonComponent
                onLoaded: { item.text = "Action"; item.clicked.connect(function() { gallery.activationCount++ }) }
            }
            Loader {
                id: iconButton
                sourceComponent: runixButtonComponent
                onLoaded: {
                    item.text = "+"
                    item.iconOnly = true
                    item.Accessible.name = "Add"
                    item.clicked.connect(function() { gallery.activationCount++ })
                }
            }
            Loader {
                id: destructiveButton
                sourceComponent: runixButtonComponent
                onLoaded: {
                    item.text = "Delete"
                    item.destructive = true
                    item.clicked.connect(function() { gallery.activationCount++ })
                }
            }
            Loader {
                id: busyButton
                sourceComponent: runixButtonComponent
                onLoaded: { item.text = "Working"; item.busy = true; item.enabled = false }
            }
            Loader {
                id: disabledButton
                sourceComponent: runixButtonComponent
                onLoaded: { item.text = "Disabled"; item.enabled = false }
            }
        }
        Label { text: "Gallery activations: " + gallery.activationCount }
        Label { text: "Checkboxes"; font.bold: true }
        GridLayout {
            columns: gallery.availableWidth < 700 ? 2 : 4
            Loader { id: checkedBox; sourceComponent: runixCheckBoxComponent; onLoaded: { item.text = "Checked"; item.checked = true } }
            Loader { id: uncheckedBox; sourceComponent: runixCheckBoxComponent; onLoaded: item.text = "Unchecked" }
            Loader { id: disabledBox; sourceComponent: runixCheckBoxComponent; onLoaded: { item.text = "Disabled checked"; item.checked = true; item.enabled = false } }
            Loader { id: disabledUncheckedBox; sourceComponent: runixCheckBoxComponent; onLoaded: { item.text = "Disabled unchecked"; item.enabled = false } }
        }
        Label { text: "Radio buttons"; font.bold: true }
        GridLayout {
            columns: gallery.availableWidth < 700 ? 2 : 4
            Loader { id: checkedRadio; sourceComponent: runixRadioButtonComponent; onLoaded: { item.text = "Selected"; item.checked = true; item.ButtonGroup.group = radioChoices } }
            Loader { id: uncheckedRadio; sourceComponent: runixRadioButtonComponent; onLoaded: { item.text = "Unselected"; item.ButtonGroup.group = radioChoices } }
            Loader { id: disabledRadio; sourceComponent: runixRadioButtonComponent; onLoaded: { item.text = "Disabled selected"; item.checked = true; item.enabled = false; item.autoExclusive = false } }
            Loader { id: disabledUncheckedRadio; sourceComponent: runixRadioButtonComponent; onLoaded: { item.text = "Disabled unselected"; item.enabled = false } }
        }
        Label { text: "Switches"; font.bold: true }
        GridLayout {
            columns: gallery.availableWidth < 700 ? 2 : 4
            Loader { id: onSwitch; sourceComponent: runixSwitchComponent; onLoaded: { item.text = "On"; item.checked = true } }
            Loader { id: offSwitch; sourceComponent: runixSwitchComponent; onLoaded: item.text = "Off" }
            Loader { id: disabledSwitch; sourceComponent: runixSwitchComponent; onLoaded: { item.text = "Disabled on"; item.checked = true; item.enabled = false } }
            Loader { id: disabledOffSwitch; sourceComponent: runixSwitchComponent; onLoaded: { item.text = "Disabled off"; item.enabled = false } }
        }
        Label { text: "Slider: " + (slider.item ? slider.item.value.toFixed(0) : "") }
        Loader {
            id: slider
            Layout.fillWidth: true
            sourceComponent: runixSliderComponent
            onLoaded: { item.from = 0; item.to = 100; item.stepSize = 1; item.value = 65 }
        }
        Label { text: "Disabled slider: 35" }
        Loader {
            id: disabledSlider
            Layout.fillWidth: true
            sourceComponent: runixSliderComponent
            onLoaded: { item.from = 0; item.to = 100; item.value = 35; item.enabled = false }
        }
        Label { text: "Selection and menus"; font.bold: true }
        RowLayout {
            Layout.fillWidth: true
            Loader {
                id: selectionBox
                sourceComponent: runixComboBoxComponent
                onLoaded: { item.textRole = "text"; item.model = [{text: "Choose an action"}, {text: "Open item"}, {text: "Copy link"}, {text: "Move to folder"}, {text: "Archive item"}, {text: "View details"}, {text: "Share item"}, {text: "Export item"}]; item.currentIndex = 0; item.popup.height = 144; item.activated.connect(function() { gallery.activationCount++ }) }
            }
            Loader { id: disabledSelectionBox; sourceComponent: runixComboBoxComponent; onLoaded: { item.textRole = "text"; item.model = [{text: "Unavailable choice"}]; item.enabled = false } }
        }
        Loader {
            id: actionMenuButton
            sourceComponent: runixButtonComponent
            onLoaded: {
                item.text = "Open action menu"
                item.clicked.connect(function() {
                    item.forceActiveFocus(Qt.TabFocusReason)
                    actionMenu.item.x = item.mapToItem(gallery, 0, 0).x
                    actionMenu.item.y = item.mapToItem(gallery, 0, item.height).y
                    actionMenu.item.open()
                })
            }
        }
        Loader {
            id: actionMenu
            sourceComponent: runixMenuComponent
            onLoaded: {
                item.parent = gallery
                item.width = 240
                for (const properties of [
                    {text: "Open"}, {text: "Mark as read", checkable: true, checked: true},
                    {text: "Unavailable", enabled: false}, {text: "Delete", destructive: true}
                ]) {
                    const row = runixMenuItemComponent.createObject(item.contentItem, properties)
                    row.triggered.connect(function() { gallery.activationCount++ })
                    item.addItem(row)
                }
            }
        }
        Label { text: "Dialog, popover, and tooltip"; font.bold: true }
        RowLayout {
            Loader {
                id: dialogButton
                sourceComponent: runixButtonComponent
                onLoaded: {
                    item.text = "Show dialog"
                    item.clicked.connect(function() { item.forceActiveFocus(Qt.TabFocusReason); dialogPopup.item.open() })
                }
            }
            Loader {
                id: popoverButton
                sourceComponent: runixButtonComponent
                onLoaded: {
                    item.text = "Show popover"
                    item.clicked.connect(function() { item.forceActiveFocus(Qt.TabFocusReason); popoverPopup.item.open() })
                }
            }
            Loader {
                id: tooltipButton
                sourceComponent: runixButtonComponent
                onLoaded: {
                    item.text = "Tooltip"
                    item.clicked.connect(function() { item.forceActiveFocus(Qt.TabFocusReason) })
                    tooltipPopup.item = runixToolTipComponent.createObject(item, {
                        parent: item, text: "The same themed hint for pointer and keyboard focus", delay: 0, timeout: -1
                    })
                    tooltipPopup.item.visible = Qt.binding(function() { return item.hovered || item.visualFocus })
                }
            }
        }
        QtObject { id: tooltipPopup; property var item: null }
        Loader {
            id: dialogPopup
            sourceComponent: runixPopupComponent
            onLoaded: {
                item.parent = gallery
                item.modalScrim = true
                item.width = 320
                item.x = Qt.binding(function() { return Math.max(0, (gallery.width - item.width) / 2) })
                item.y = Qt.binding(function() { return Math.max(0, (gallery.height - item.height) / 2) })
                item.height = 120
                item.contentItem = dialogContent.createObject(item)
            }
        }
        Loader {
            id: popoverPopup
            sourceComponent: runixPopupComponent
            onLoaded: {
                item.parent = gallery
                item.width = 280
                item.x = Qt.binding(function() { return Math.max(0, (gallery.width - item.width) / 2) })
                item.y = Qt.binding(function() { return Math.max(0, (gallery.height - item.height) / 2) })
                item.height = 100
                item.contentItem = popoverContent.createObject(item)
            }
        }
        Component {
            id: dialogContent
            Column {
                spacing: 8
                Text { text: "Dialog specimen"; textFormat: Text.PlainText; color: activeAppearance.foregrounds.primary }
                Text {
                    width: 296
                    wrapMode: Text.Wrap
                    textFormat: Text.PlainText
                    text: "A themed modal surface with focus restoration."
                    color: activeAppearance.foregrounds.secondary
                }
                Loader { sourceComponent: runixButtonComponent; onLoaded: { item.text = "Close"; item.clicked.connect(function() { dialogPopup.item.close() }) } }
            }
        }
        Component {
            id: popoverContent
            Column {
                spacing: 8
                Text { text: "Popover specimen"; color: activeAppearance.foregrounds.primary }
                Text { text: "A non-modal B1 surface."; color: activeAppearance.foregrounds.secondary }
                Loader { sourceComponent: runixButtonComponent; onLoaded: { item.text = "Close"; item.clicked.connect(function() { popoverPopup.item.close() }) } }
            }
        }
        Label { text: "Shortcut and supporting specimens"; font.bold: true }
        GridLayout {
            columns: 2
            Loader { id: keycapSample; sourceComponent: runixSupportingComponent; onLoaded: { item.kind = "keycap"; item.text = "Ctrl" } }
            Loader { id: recorderSample; sourceComponent: runixSupportingComponent; onLoaded: { item.kind = "recorder"; item.text = "Record shortcut" } }
            Loader { id: tagSample; sourceComponent: runixSupportingComponent; onLoaded: { item.kind = "tag"; item.text = "Featured"; item.hue = "purple" } }
            Loader { id: sectionSample; sourceComponent: runixSupportingComponent; onLoaded: { item.kind = "section"; item.text = "Recent items"; item.detail = "6" } }
            Loader { id: statusSample; sourceComponent: runixSupportingComponent; onLoaded: { item.kind = "status"; item.text = "Connected"; item.hue = "green" } }
            Loader { id: iconSample; sourceComponent: runixSupportingComponent; onLoaded: { item.kind = "glyph"; item.text = "◆"; item.detail = "Monochrome UI glyph" } }
            Loader { id: emptySample; sourceComponent: runixSupportingComponent; onLoaded: { item.kind = "empty"; item.text = "Nothing here yet"; item.detail = "Items will appear when available." } }
            Loader { id: loadingSample; sourceComponent: runixSupportingComponent; onLoaded: { item.kind = "loading"; item.text = "Loading items"; item.detail = "Please wait" } }
            Loader { sourceComponent: runixSupportingComponent; onLoaded: { item.kind = "status"; item.text = "Warning"; item.detail = "Review this value"; item.hue = "yellow" } }
            Loader { sourceComponent: runixSupportingComponent; onLoaded: { item.kind = "status"; item.text = "Error"; item.detail = "The sample action failed"; item.hue = "red" } }
            Loader { sourceComponent: runixSupportingComponent; onLoaded: { item.kind = "status"; item.text = "Information"; item.detail = "No action is needed"; item.hue = "blue" } }
        }
        Label { text: "Control and sampled contrast inspection"; font.bold: true }
        Loader {
            id: controlPickerLoader
            Layout.fillWidth: true
            sourceComponent: runixComboBoxComponent
            onLoaded: {
                item.model = Qt.binding(function() { return Object.keys(gallery.controls) })
                item.currentIndex = Qt.binding(function() { return item.model.indexOf(gallery.selectedID) })
                item.activated.connect(function() { gallery.selectedID = item.currentText })
            }
        }
        Loader {
            id: rolePickerLoader
            Layout.fillWidth: true
            sourceComponent: runixComboBoxComponent
            onLoaded: {
                item.model = Qt.binding(function() {
                    return activeAppearance.diagnostics.map(function(role) { return role.name })
                })
                item.currentIndex = Qt.binding(function() { return item.model.indexOf(gallery.selectedRole) })
                item.activated.connect(function() { gallery.selectedRole = item.currentText })
            }
        }

        Label {
            id: inspection
            Layout.fillWidth: true
            wrapMode: Text.Wrap
            textFormat: Text.PlainText
            text: {
                const control = gallery.selectedControl
                if (!control) return "Select a control"
                const state = gallery.selectedID === "gallery-password" ? "[redacted]" :
                    JSON.stringify(gallery.inspect(control)) + "\nValue: " +
                    (control.text === undefined ? control.value : control.text)
                const role = activeAppearance.diagnostics.find(function(role) { return role.name === gallery.selectedRole })
                if (!role) return state
                const d = role.diagnostic
                return "Control: " + gallery.selectedID + "\n" + state + "\nRole: " + role.name
                    + "\nOriginal: " + JSON.stringify(role.original) + "\nEffective: " + JSON.stringify(role.effective)
                    + "\nWorst sampled background: " + JSON.stringify(d.worst.color)
                    + "\nContext: " + d.worst.context + " · y=" + d.worst.y
                    + "\nSampled minimum: " + d.minimum.toFixed(3) + " / target " + d.target
                    + " · " + (d.passing ? "passing" : "below target")
                    + " · correction " + (d.changed ? "changed" : "unchanged")
            }
        }
    }
    Component.onCompleted: {
        const entries = [
            ["gallery-search", searchField, "textbox", "Search", false],
            ["gallery-text", plainField, "textbox", "Text", false],
            ["gallery-password", passwordField, "password", "Password", true],
            ["gallery-error", errorField, "textbox", "Validation", false],
            ["gallery-readonly", readOnlyField, "textbox", "Read-only text", false],
            ["gallery-disabled", disabledField, "textbox", "Disabled text", false],
            ["gallery-notes", notes, "multiline", "Notes", false],
            ["gallery-notes-error", errorNotes, "multiline", "Notes validation", false],
            ["gallery-notes-readonly", readOnlyNotes, "multiline", "Read-only notes", false],
            ["gallery-notes-disabled", disabledNotes, "multiline", "Disabled notes", false],
            ["gallery-action", actionButton, "button", "Action", false],
            ["gallery-icon", iconButton, "button", "Add", false],
            ["gallery-destructive", destructiveButton, "button", "Delete", false],
            ["gallery-busy", busyButton, "button", "Working", false],
            ["gallery-disabled-button", disabledButton, "button", "Disabled button", false],
            ["gallery-checked", checkedBox, "checkbox", "Checked", false],
            ["gallery-unchecked", uncheckedBox, "checkbox", "Unchecked", false],
            ["gallery-disabled-checked", disabledBox, "checkbox", "Disabled checked", false],
            ["gallery-disabled-unchecked", disabledUncheckedBox, "checkbox", "Disabled unchecked", false],
            ["gallery-radio-selected", checkedRadio, "radio", "Selected", false],
            ["gallery-radio-unselected", uncheckedRadio, "radio", "Unselected", false],
            ["gallery-radio-disabled", disabledRadio, "radio", "Disabled selected", false],
            ["gallery-radio-disabled-unselected", disabledUncheckedRadio, "radio", "Disabled unselected", false],
            ["gallery-switch-on", onSwitch, "switch", "On", false],
            ["gallery-switch-off", offSwitch, "switch", "Off", false],
            ["gallery-switch-disabled", disabledSwitch, "switch", "Disabled on", false],
            ["gallery-switch-disabled-off", disabledOffSwitch, "switch", "Disabled off", false],
            ["gallery-slider", slider, "slider", "Slider", false],
            ["gallery-slider-disabled", disabledSlider, "slider", "Disabled slider", false],
            ["gallery-selection", selectionBox, "combobox", "Selection", false],
            ["gallery-selection-disabled", disabledSelectionBox, "combobox", "Disabled selection", false],
            ["gallery-menu", actionMenuButton, "button", "Action menu", false],
            ["gallery-dialog", dialogButton, "button", "Dialog", false],
            ["gallery-popover", popoverButton, "button", "Popover", false],
            ["gallery-tooltip", tooltipButton, "button", "Tooltip", false],
            ["gallery-keycap", keycapSample, "status", "Shortcut keycap", false],
            ["gallery-recorder", recorderSample, "status", "Shortcut recorder", false],
            ["gallery-tag", tagSample, "status", "Tag", false],
            ["gallery-section", sectionSample, "status", "Section header", false],
            ["gallery-status", statusSample, "status", "Status", false],
            ["gallery-icon-sample", iconSample, "status", "Monochrome icon", false],
            ["gallery-empty", emptySample, "status", "Empty state", false],
            ["gallery-loading", loadingSample, "status", "Loading state", false]
        ]
        const registered = ({})
        for (const entry of entries) {
            const control = entry[1].item
            registered[entry[0]] = control
            if (typeof semanticDebug === "undefined") continue
            const button = ["button", "checkbox", "radio", "switch"].indexOf(entry[2]) >= 0
            const actions = button ? ["focus", "activate", "key"] : entry[2] === "slider" ? ["focus", "key"]
                : entry[2] === "combobox" ? ["focus", "select", "key"]
                : control.kind === "recorder" ? ["focus", "key"] : entry[2] === "status" ? []
                : control.readOnly ? ["focus", "select", "key"] : ["focus", "replace", "select", "key"]
            semanticDebug.registerControl(entry[0], control, entry[2], entry[3], actions,
                button ? function() { control.click() } : null, entry[4])
            semanticDebug.controls[entry[0]].inspect = function() { return gallery.inspect(control) }
        }
        const focusOrder = entries.map(function(entry) { return entry[1].item })
            .filter(function(control) { return control.enabled && (control.activeFocusOnTab || control.kind === "recorder") })
        focusOrder.push(controlPicker, rolePicker)
        for (let i = 0; i < focusOrder.length; ++i) {
            const control = focusOrder[i]
            control.KeyNavigation.priority = KeyNavigation.BeforeItem
            control.KeyNavigation.tab = focusOrder[(i + 1) % focusOrder.length]
            control.KeyNavigation.backtab = focusOrder[(i + focusOrder.length - 1) % focusOrder.length]
        }
        controls = registered
        if (typeof semanticDebug === "undefined") return
        semanticDebug.registerControl("gallery-scroll", gallery, "scroll", "Gallery specimens", ["scroll"], null, false)
        const bar = gallery.ScrollBar.vertical
        semanticDebug.registerControl("gallery-scrollbar", bar, "scrollbar", "Gallery scrollbar", [], null, false)
        semanticDebug.controls["gallery-scrollbar"].inspect = function() {
            return {position: bar.position, size: bar.size, hovered: bar.hovered, pressed: bar.pressed}
        }
        semanticDebug.controls["gallery-scroll"].scroll = function(id) {
            if (id === "gallery-inspection") { gallery.reveal(inspection); return true }
            if (!Object.prototype.hasOwnProperty.call(gallery.controls, id)) return false
            gallery.reveal(gallery.controls[id])
            return true
        }
        semanticDebug.registerControl("gallery-inspection", inspection, "status", "Control inspection", [], null, false)
        for (const pair of [["gallery-control", controlPicker], ["gallery-role", rolePicker]]) {
            const picker = pair[1]
            semanticDebug.registerControl(pair[0], picker, "combobox", pair[0], ["focus", "select"], null, false)
            semanticDebug.controls[pair[0]].select = function(value) {
                const index = picker.model.indexOf(value)
                if (index < 0) return false
                picker.currentIndex = index
                picker.activated(index)
                return true
            }
        }
    }
    Instantiator {
        model: Object.keys(gallery.controls)
        delegate: Connections {
            target: gallery.controls[modelData]
            function onActiveFocusChanged() {
                if (!target.activeFocus) return
                gallery.selectedID = modelData
                gallery.reveal(target)
            }
        }
    }
}
