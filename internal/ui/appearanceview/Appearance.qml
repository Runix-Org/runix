import QtQuick

QtObject {
    id: root
    property var appearanceState: appearanceStateModel
    property int revision: 0
    function rowIndex() { return appearanceState.index(0, 0) }
    function command(value) {
        return appearanceState.setData(rowIndex(), JSON.stringify(value), 258)
    }
    readonly property var snapshot: {
        revision
        return JSON.parse(appearanceState.data(rowIndex(), 257))
    }
    readonly property var preferences: snapshot.preferences || ({})
    readonly property string activeID: snapshot.active_id || ""
    readonly property string name: snapshot.name || ""
    readonly property string scheme: snapshot.scheme || "unknown"
    readonly property string fallbackReason: snapshot.fallback_reason || ""
    readonly property string error: snapshot.error || ""
    readonly property var roles: snapshot.roles || ({})
    readonly property var foregrounds: {
        const result = ({})
        const list = roles.foregrounds || []
        for (let i = 0; i < list.length; ++i) {
            const item = list[i], c = item.effective
            result[item.name] = Qt.rgba(c.r, c.g, c.b, c.a)
        }
        return result
    }
    readonly property var originalForegrounds: {
        const result = ({})
        const list = roles.foregrounds || []
        for (let i = 0; i < list.length; ++i) {
            const item = list[i], c = item.original
            result[item.name] = Qt.rgba(c.r, c.g, c.b, c.a)
        }
        return result
    }
    readonly property var surfaces: {
        const source = roles.surfaces || ({}), result = ({})
        for (const key in source) {
            const c = source[key]
            if (c && c.r !== undefined)
                result[key] = Qt.rgba(c.r, c.g, c.b, c.a)
        }
        return result
    }
    readonly property var diagnostics: roles.foregrounds || []
    readonly property var lightThemes: snapshot.light_themes || []
    readonly property var darkThemes: snapshot.dark_themes || []
    function setPreferences(value) { return command({op: "preferences", preferences: value}) }
    function setScheme(value) { return command({op: "scheme", scheme: value}) }
    function observeScheme(scheme) {
        if (scheme === Qt.ColorScheme.Light)
            setScheme("light")
        else if (scheme === Qt.ColorScheme.Dark)
            setScheme("dark")
        else
            setScheme("unknown")
    }
    function refresh() { revision++ }
    property Connections modelChanges: Connections {
        target: appearanceState
        function onDataChanged() { root.refresh() }
    }
    property Connections schemeChanges: Connections {
        target: Qt.styleHints
        function onColorSchemeChanged(colorScheme) { root.observeScheme(colorScheme) }
    }
    Component.onCompleted: observeScheme(Qt.styleHints.colorScheme)
}
