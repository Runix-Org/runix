import QtQuick

QtObject {
    readonly property color text: activeAppearance.foregrounds.primary
    readonly property color secondary: activeAppearance.foregrounds.secondary
    readonly property color placeholder: activeAppearance.foregrounds.placeholder
    readonly property color disabled: activeAppearance.foregrounds.disabled
    readonly property color focus: activeAppearance.foregrounds.focus
    readonly property color error: activeAppearance.foregrounds["red-text"]
    readonly property color icon: activeAppearance.foregrounds["monochrome-icon"]
    readonly property color errorIcon: activeAppearance.foregrounds["red-icon"]
    readonly property color selectedText: activeAppearance.foregrounds["selected-text"]
    readonly property color onChecked: activeAppearance.foregrounds["on-checked"]
    readonly property color unchecked: activeAppearance.foregrounds["unchecked-outline"]
    readonly property color caret: activeAppearance.foregrounds.caret
    readonly property color loader: activeAppearance.foregrounds.loader
    readonly property color sliderFill: activeAppearance.foregrounds["slider-fill"]
    readonly property color sliderThumb: activeAppearance.foregrounds["slider-thumb"]
    readonly property color switchThumb: activeAppearance.foregrounds["switch-thumb"]
    readonly property color surface: activeAppearance.surfaces.field
    readonly property color fieldBorder: activeAppearance.surfaces.border
    readonly property color hoverBorder: activeAppearance.surfaces.hover_border
    readonly property color separator: activeAppearance.surfaces.separator
    readonly property color disabledBorder: activeAppearance.surfaces.disabled_border
    readonly property color button: activeAppearance.surfaces.button
    readonly property color selectedFill: activeAppearance.surfaces.checked
    readonly property color track: activeAppearance.surfaces.track
    readonly property color disabledTrack: activeAppearance.surfaces.disabled_track
    readonly property color hover: activeAppearance.surfaces.hover
    readonly property color press: activeAppearance.surfaces.press
    readonly property int radius: 5
    readonly property int spacing: 8

    function foreground(control) { return control.enabled ? text : disabled }

    function focusVisible(control) { return control.visualFocus }

    function wash(control) {
        return !control.enabled ? "transparent"
            : control.pressed ? press : control.hovered ? hover : "transparent"
    }

}
