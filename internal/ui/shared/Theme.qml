import QtQuick

QtObject {
    readonly property color foreground: activeAppearance.foregrounds.primary
    readonly property color muted: activeAppearance.foregrounds.secondary
    readonly property color surface: activeAppearance.surfaces.b1
    readonly property color field: activeAppearance.surfaces.b1
    readonly property color accent: activeAppearance.foregrounds.focus
    readonly property color error: activeAppearance.foregrounds["red-text"]
    readonly property int spacing: 8
    readonly property int radius: 5
}
