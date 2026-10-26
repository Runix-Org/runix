import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Item {
    id: root
    property var state: null
    readonly property var theme: themeLoader.item
    readonly property alias initialFocus: nameField
    signal dismissed()
    readonly property var info: state ? JSON.parse(state.snapshot) : ({})

    function command(value) {
        if (state)
            state.command = JSON.stringify(value)
    }
    function edit() {
        command({op: "edit", values: {name: nameField.text, notes: notesField.text,
            password: passwordField.text, choice: choiceBox.currentIndex === 1 ? "Personal"
                : choiceBox.currentIndex === 2 ? "Work" : "", accepted: acceptedBox.checked}})
    }
    function cancel() {
        command({op: "cancel"})
        passwordField.clear()
    }
    function submit() {
        if (info.busy) return
        const moveFocus = submitButton.activeFocus
        edit()
        command({op: "submit"})
        if (info.busy && moveFocus) cancelButton.forceActiveFocus(Qt.TabFocusReason)
    }
    function reveal(item) {
        Qt.callLater(function() {
            const view = scroll.contentItem
            const point = item.mapToItem(view.contentItem, 0, 0)
            let y = view.contentY
            if (point.y < y) y = point.y
            else if (point.y + item.height > y + view.height) y = point.y + item.height - view.height
            view.contentY = Math.max(0, Math.min(y, Math.max(0, view.contentHeight - view.height)))
        })
    }
    function register(id, item, role, label, actions, activate, secret) {
        if (typeof semanticDebug !== "undefined")
            semanticDebug.registerControl(id, item, role, label, actions, activate, secret)
    }

    Component.onCompleted: {
        register("form-name", nameField, "textbox", "Name", ["focus", "replace", "select", "key"], null, false)
        register("form-notes", notesField, "textbox", "Notes", ["focus", "replace", "select"], null, false)
        register("form-password", passwordField, "password", "Password", ["focus", "replace", "select"], null, true)
        register("form-accepted", acceptedBox, "checkbox", "Accepted", ["focus", "activate", "key"], function() { acceptedBox.checked = !acceptedBox.checked }, false)
        register("form-choice", choiceBox, "combobox", "Choice", ["focus", "activate", "key"], function() { choiceBox.popup.open() }, false)
        register("form-submit", submitButton, "button", "Submit", ["focus", "activate", "key"], function() { root.submit() }, false)
        register("form-cancel", cancelButton, "button", "Cancel", ["focus", "activate", "key"], function() { root.cancel(); root.dismissed() }, false)
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

    Loader { id: themeLoader; sourceComponent: labThemeComponent }
    Rectangle { anchors.fill: parent; color: theme.surface; radius: theme.radius }
    ScrollView {
        id: scroll
        anchors.fill: parent
        contentWidth: availableWidth
        clip: true
    ColumnLayout {
        width: parent.width
        spacing: theme.spacing
        TextField {
            id: nameField
            onActiveFocusChanged: if (activeFocus) root.reveal(nameField)
            Layout.fillWidth: true
            placeholderText: "Name"
            onTextChanged: root.edit()
            KeyNavigation.tab: notesField
            KeyNavigation.backtab: cancelButton
            palette.text: theme.foreground
            palette.placeholderText: theme.muted
            background: Rectangle { color: theme.field; radius: theme.radius; border.color: nameField.activeFocus ? theme.accent : theme.muted; border.width: nameField.activeFocus ? 2 : 1 }
            onAccepted: if (!inputMethodComposing) root.submit()
        }
        Label { id: nameError; Layout.fillWidth: true; visible: !!root.info.errors?.name; text: root.info.errors?.name || ""; color: theme.error }
        TextArea {
            id: notesField
            onActiveFocusChanged: if (activeFocus) root.reveal(notesField)
            Layout.fillWidth: true
            Layout.preferredHeight: 76
            placeholderText: "Notes"
            wrapMode: TextEdit.Wrap
            KeyNavigation.priority: KeyNavigation.BeforeItem
            onTextChanged: root.edit()
            KeyNavigation.tab: passwordField
            KeyNavigation.backtab: nameField
            palette.text: theme.foreground
            palette.placeholderText: theme.muted
            background: Rectangle { color: theme.field; radius: theme.radius; border.color: notesField.activeFocus ? theme.accent : theme.muted; border.width: notesField.activeFocus ? 2 : 1 }
        }
        Label { id: notesError; Layout.fillWidth: true; visible: !!root.info.errors?.notes; text: root.info.errors?.notes || ""; color: theme.error }
        TextField {
            id: passwordField
            onActiveFocusChanged: if (activeFocus) root.reveal(passwordField)
            Layout.fillWidth: true
            placeholderText: "Password"
            echoMode: TextInput.Password
            onTextChanged: root.edit()
            KeyNavigation.tab: acceptedBox
            KeyNavigation.backtab: notesField
            palette.text: theme.foreground
            palette.placeholderText: theme.muted
            background: Rectangle { color: theme.field; radius: theme.radius; border.color: passwordField.activeFocus ? theme.accent : theme.muted; border.width: passwordField.activeFocus ? 2 : 1 }
            onAccepted: if (!inputMethodComposing) root.submit()
        }
        Label { id: passwordError; Layout.fillWidth: true; visible: !!root.info.errors?.password; text: root.info.errors?.password || ""; color: theme.error }
        CheckBox {
            id: acceptedBox
            onActiveFocusChanged: if (activeFocus) root.reveal(acceptedBox)
            text: "I accept"
            onCheckedChanged: root.edit()
            KeyNavigation.tab: choiceBox
            KeyNavigation.backtab: passwordField
        }
        Label { id: acceptedError; Layout.fillWidth: true; visible: !!root.info.errors?.accepted; text: root.info.errors?.accepted || ""; color: theme.error }
        ComboBox {
            id: choiceBox
            onActiveFocusChanged: if (activeFocus) root.reveal(choiceBox)
            Layout.fillWidth: true
            model: ["Choose…", "Personal", "Work"]
            onActivated: root.edit()
            KeyNavigation.tab: root.info.busy ? cancelButton : submitButton
            KeyNavigation.backtab: acceptedBox
            palette.text: theme.foreground
            background: Rectangle { color: theme.field; radius: theme.radius; border.color: choiceBox.activeFocus ? theme.accent : theme.muted; border.width: choiceBox.activeFocus ? 2 : 1 }
        }
        Label { id: choiceError; Layout.fillWidth: true; visible: !!root.info.errors?.choice; text: root.info.errors?.choice || ""; color: theme.error }
        RowLayout {
            Layout.fillWidth: true
            Button {
                id: submitButton
                onActiveFocusChanged: if (activeFocus) root.reveal(submitButton)
                text: root.info.busy ? "Submitting…" : "Submit"
                enabled: !root.info.busy
                onClicked: root.submit()
                KeyNavigation.tab: cancelButton
                KeyNavigation.backtab: choiceBox
            }
            Button {
                id: cancelButton
                onActiveFocusChanged: if (activeFocus) root.reveal(cancelButton)
                text: "Cancel"
                onClicked: { root.cancel(); root.dismissed() }
                KeyNavigation.tab: nameField
                KeyNavigation.backtab: root.info.busy ? choiceBox : submitButton
            }
        }
        Label { id: statusLabel; Layout.fillWidth: true; text: root.info.status || "Ready"; textFormat: Text.PlainText }
    }
    }
}
