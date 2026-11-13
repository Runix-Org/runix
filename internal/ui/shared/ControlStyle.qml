import QtQuick

QtObject {
    readonly property color text: activeAppearance.foregrounds.primary
    readonly property color secondary: activeAppearance.foregrounds.secondary
    readonly property color placeholder: activeAppearance.foregrounds.placeholder
    readonly property color disabled: activeAppearance.foregrounds.disabled
    readonly property color focus: activeAppearance.foregrounds.focus
    readonly property color selectionMarker: activeAppearance.foregrounds["selection-marker"]
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
    readonly property color selectedRow: activeAppearance.surfaces.selected
    readonly property color track: activeAppearance.surfaces.track
    readonly property color disabledTrack: activeAppearance.surfaces.disabled_track
    readonly property color hover: activeAppearance.surfaces.hover
    readonly property color press: activeAppearance.surfaces.press
    readonly property color scrim: activeAppearance.surfaces.scrim
    readonly property color popup: activeAppearance.surfaces.b1
    readonly property color popupBorder: activeAppearance.surfaces.border
    readonly property color scrollThumb: activeAppearance.surfaces.scrollbar
    readonly property color scrollHover: activeAppearance.surfaces.scrollbar_hover
    readonly property color scrollPressed: activeAppearance.surfaces.scrollbar_pressed
    function hueIndex(hue) {
        const names = ["red", "orange", "yellow", "green", "blue", "purple", "magenta"]
        const index = names.indexOf(hue)
        return index < 0 ? 4 : index
    }
    function hueText(hue) { return activeAppearance.foregrounds[hue + "-text"] || text }
    function hueIcon(hue) { return activeAppearance.foregrounds[hue + "-icon"] || icon }
    function tagFill(hue) { return activeAppearance.surfaces["tag_" + hueIndex(hue)] || button }

    readonly property int radius: 5
    readonly property int spacing: 8

    // Logical units; Qt's existing interface scale applies exactly once.
    readonly property int contentInset: 16
    readonly property int formLabelWidth: 120
    readonly property int formControlMinWidth: 280
    readonly property int formColumnGap: 16
    readonly property int formRowSpacing: 12
    readonly property int fieldHeight: 30
    readonly property int textAreaHeight: 76
    readonly property int headingSize: 18
    readonly property int titleSize: 20
    readonly property int sectionSpacing: 24

    function foreground(control) { return control.enabled ? text : disabled }

    function focusVisible(control) { return control.visualFocus }

    function wash(control) {
        return !control.enabled ? "transparent"
            : control.pressed ? press : control.hovered ? hover : "transparent"
    }

}
