import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Item {
    id: root
    property var state: null
    readonly property alias initialFocus: nameField.item
    signal dismissed()
    readonly property var info: state ? JSON.parse(state.snapshot) : ({})
    function command(value) { if (state) state.command = JSON.stringify(value) }
    function edit() {
        if (!nameField.item || !notesField.item || !passwordField.item || !choiceBox.item || !acceptedBox.item) return
        command({op: "edit", values: {name: nameField.item.text, notes: notesField.item.text,
            password: passwordField.item.text, choice: choiceBox.item.currentIndex === 1 ? "Personal"
                : choiceBox.item.currentIndex === 2 ? "Work" : "", accepted: acceptedBox.item.checked}})
    }
    function cancel() { command({op: "cancel"}); passwordField.item.clear() }
    function submit() {
        if (info.busy) return
        const moveFocus = submitButton.item.activeFocus
        edit()
        command({op: "submit"})
        if (info.busy && moveFocus) cancelButton.item.forceActiveFocus(Qt.TabFocusReason)
    }
    function reveal(item) {
        Qt.callLater(function() {
            const view = scroll.contentItem
            const point = item.mapToItem(view.contentItem, 0, 0)
            let y = view.contentY
            if (point.y - 4 < y) y = point.y - 4
            else if (point.y + item.height + 4 > y + view.height) y = point.y + item.height + 4 - view.height
            view.contentY = Math.max(0, Math.min(y, Math.max(0, view.contentHeight - view.height)))
        })
    }
    function register(id, item, role, label, actions, activate, secret) {
        if (typeof semanticDebug !== "undefined") semanticDebug.registerControl(id, item, role, label, actions, activate, secret)
    }
    Component.onCompleted: {
        const order = [nameField.item, notesField.item, passwordField.item, acceptedBox.item,
            choiceBox.item, submitButton.item, cancelButton.item]
        for (let i = 0; i < order.length; ++i) {
            const control = order[i]
            control.KeyNavigation.priority = KeyNavigation.BeforeItem
            control.KeyNavigation.tab = order[(i + 1) % order.length]
            control.KeyNavigation.backtab = order[(i + order.length - 1) % order.length]
        }
        choiceBox.item.KeyNavigation.tab = Qt.binding(function() {
            return root.info.busy ? cancelButton.item : submitButton.item
        })
        cancelButton.item.KeyNavigation.backtab = Qt.binding(function() {
            return root.info.busy ? choiceBox.item : submitButton.item
        })
        register("form-name", nameField.item, "textbox", "Name", ["focus", "replace", "select", "key"], null, false)
        register("form-notes", notesField.item, "textbox", "Notes", ["focus", "replace", "select"], null, false)
        register("form-password", passwordField.item, "password", "Password", ["focus", "replace", "select"], null, true)
        register("form-accepted", acceptedBox.item, "checkbox", "Accepted", ["focus", "activate", "key"], function() { acceptedBox.item.checked = !acceptedBox.item.checked }, false)
        register("form-choice", choiceBox.item, "combobox", "Choice", ["focus", "activate", "key"], function() { choiceBox.item.popup.open() }, false)
        register("form-submit", submitButton.item, "button", "Submit", ["focus", "activate", "key"], function() { root.submit() }, false)
        register("form-cancel", cancelButton.item, "button", "Cancel", ["focus", "activate", "key"], function() { root.cancel(); root.dismissed() }, false)
        register("form-status", statusLabel, "status", "Form status", [], null, false)
        register("form-error-name", nameError, "status", "Name error", [], null, false)
        register("form-error-notes", notesError, "status", "Notes error", [], null, false)
        register("form-error-password", passwordError, "status", "Password error", [], null, false)
        register("form-error-choice", choiceError, "status", "Choice error", [], null, false)
        register("form-error-accepted", acceptedError, "status", "Accepted error", [], null, false)
    }
    onInfoChanged: {
        if (typeof semanticDebug !== "undefined" && semanticDebug.controls["form-status"])
            semanticDebug.controls["form-status"].inspect = function() {
                return {revision: root.info.revision, submitted_revision: root.info.submitted_revision,
                    busy: root.info.busy, status: root.info.status, errors: root.info.errors}
            }
    }
    readonly property bool wide: scroll.availableWidth - runixControlStyle.spacing * 2
        >= runixControlStyle.formLabelWidth + runixControlStyle.formColumnGap + runixControlStyle.formControlMinWidth
    component FieldLabel: Label {
        Layout.preferredWidth: root.wide ? runixControlStyle.formLabelWidth : -1
        Layout.fillWidth: !root.wide
        Layout.alignment: Qt.AlignTop
        Layout.topMargin: root.wide ? runixControlStyle.spacing : 0
        color: runixControlStyle.secondary
        textFormat: Text.PlainText
        wrapMode: Text.Wrap
    }
    component FieldError: Label {
        Layout.fillWidth: true
        visible: text.length > 0
        color: runixControlStyle.error
        textFormat: Text.PlainText
        wrapMode: Text.Wrap
    }
    Shortcut {
        sequence: "Ctrl+Return"
        enabled: root.visible && !root.info.busy
            && !!nameField.item && !nameField.item.inputMethodComposing
            && !!notesField.item && !notesField.item.inputMethodComposing
            && !!passwordField.item && !passwordField.item.inputMethodComposing
        onActivated: root.submit()
    }
    ScrollView {
        id: scroll
        anchors.top: parent.top
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: footer.top
        anchors.margins: runixControlStyle.spacing
        contentWidth: availableWidth
        contentHeight: content.implicitHeight + runixControlStyle.spacing * 2
        clip: true
        ScrollBar.vertical: runixScrollBarComponent.createObject(null, {policy: ScrollBar.AsNeeded})
        GridLayout {
            id: content
            x: runixControlStyle.spacing
            y: runixControlStyle.spacing
            width: Math.max(0, scroll.availableWidth - runixControlStyle.spacing * 2)
            columns: root.wide ? 2 : 1
            columnSpacing: runixControlStyle.formColumnGap
            rowSpacing: runixControlStyle.formRowSpacing
            FieldLabel { text: "Name" }
            ColumnLayout {
                Layout.fillWidth: true
                Layout.minimumWidth: 0
                spacing: runixControlStyle.spacing
                Loader {
                    id: nameField
                    Layout.fillWidth: true
                    Layout.minimumWidth: 0
                    sourceComponent: runixTextFieldComponent
                    onLoaded: {
                        item.Accessible.name = "Name"
                        item.placeholderText = "Name"
                        item.errorText = Qt.binding(function() { return root.info.errors?.name || "" })
                        item.textChanged.connect(root.edit)
                        item.accepted.connect(function() { if (!item.inputMethodComposing) root.submit() })
                    }
                    Connections { target: nameField.item; function onActiveFocusChanged() { if (nameField.item.activeFocus) root.reveal(nameField.item) } }
                }
                FieldError { id: nameError; text: root.info.errors?.name ? "! " + root.info.errors.name : "" }
            }
            FieldLabel { text: "Notes" }
            ColumnLayout {
                Layout.fillWidth: true
                Layout.minimumWidth: 0
                spacing: runixControlStyle.spacing
                Loader {
                    id: notesField
                    Layout.fillWidth: true
                    Layout.minimumWidth: 0
                    Layout.preferredHeight: runixControlStyle.textAreaHeight
                    sourceComponent: runixTextAreaComponent
                    onLoaded: {
                        item.Accessible.name = "Notes"
                        item.placeholderText = "Notes"
                        item.wrapMode = TextEdit.Wrap
                        item.errorText = Qt.binding(function() { return root.info.errors?.notes || "" })
                        item.textChanged.connect(root.edit)
                    }
                    Connections { target: notesField.item; function onActiveFocusChanged() { if (notesField.item.activeFocus) root.reveal(notesField.item) } }
                }
                FieldError { id: notesError; text: root.info.errors?.notes ? "! " + root.info.errors.notes : "" }
            }
            FieldLabel { text: "Password" }
            ColumnLayout {
                Layout.fillWidth: true
                Layout.minimumWidth: 0
                spacing: runixControlStyle.spacing
                Loader {
                    id: passwordField
                    Layout.fillWidth: true
                    Layout.minimumWidth: 0
                    sourceComponent: runixTextFieldComponent
                    onLoaded: {
                        item.Accessible.name = "Password"
                        item.placeholderText = "Password"
                        item.echoMode = TextInput.Password
                        item.errorText = Qt.binding(function() { return root.info.errors?.password || "" })
                        item.textChanged.connect(root.edit)
                        item.accepted.connect(function() { if (!item.inputMethodComposing) root.submit() })
                    }
                    Connections { target: passwordField.item; function onActiveFocusChanged() { if (passwordField.item.activeFocus) root.reveal(passwordField.item) } }
                }
                FieldError { id: passwordError; text: root.info.errors?.password ? "! " + root.info.errors.password : "" }
            }
            FieldLabel { text: "Consent" }
            ColumnLayout {
                Layout.fillWidth: true
                Layout.minimumWidth: 0
                spacing: runixControlStyle.spacing
                Loader {
                    id: acceptedBox
                    sourceComponent: runixCheckBoxComponent
                    onLoaded: { item.text = "I accept"; item.checkedChanged.connect(root.edit) }
                    Connections { target: acceptedBox.item; function onActiveFocusChanged() { if (acceptedBox.item.activeFocus) root.reveal(acceptedBox.item) } }
                }
                FieldError { id: acceptedError; text: root.info.errors?.accepted ? "! " + root.info.errors.accepted : "" }
            }
            FieldLabel { text: "Choice" }
            ColumnLayout {
                Layout.fillWidth: true
                Layout.minimumWidth: 0
                spacing: runixControlStyle.spacing
                Loader {
                    id: choiceBox
                    Layout.fillWidth: true
                    Layout.minimumWidth: 0
                    sourceComponent: runixComboBoxComponent
                    onLoaded: {
                        item.Accessible.name = "Choice"
                        item.model = ["Choose…", "Personal", "Work"]
                        item.currentIndex = 0
                        item.activated.connect(root.edit)
                    }
                    Connections { target: choiceBox.item; function onActiveFocusChanged() { if (choiceBox.item.activeFocus) root.reveal(choiceBox.item) } }
                }
                FieldError { id: choiceError; text: root.info.errors?.choice ? "! " + root.info.errors.choice : "" }
            }
        }
    }
    ColumnLayout {
        id: footer
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        anchors.margins: runixControlStyle.spacing
        spacing: runixControlStyle.spacing
        Rectangle { Layout.fillWidth: true; implicitHeight: 1; color: runixControlStyle.separator }
        RowLayout {
            Layout.fillWidth: true
            Loader {
                id: submitButton
                sourceComponent: runixButtonComponent
                onLoaded: {
                    item.text = Qt.binding(function() { return root.info.busy ? "Submitting…" : "Submit" })
                    item.busy = Qt.binding(function() { return !!root.info.busy })
                    item.enabled = Qt.binding(function() { return !root.info.busy })
                    item.clicked.connect(root.submit)
                }
            }
            Loader {
                id: cancelButton
                sourceComponent: runixButtonComponent
                onLoaded: { item.text = "Cancel"; item.clicked.connect(function() { root.cancel(); root.dismissed() }) }
            }
        }
        Label {
            id: statusLabel
            Layout.fillWidth: true
            text: root.info.status || "Ready"
            textFormat: Text.PlainText
            color: runixControlStyle.secondary
            wrapMode: Text.Wrap
        }
    }
    // Focus remains on native controls; their KeyNavigation links wrap the form.
}
