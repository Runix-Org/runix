import QtQuick
import QtQuick.Window

QtObject {
    id: policy
    property bool attached: false
    property Window window
    property bool expanded: false
    readonly property size requestedSize: expanded ? Qt.size(800, 640) : Qt.size(640, 420)
    readonly property int availableWidth: window ? Math.max(1, window.Screen.desktopAvailableWidth) : 1
    readonly property int availableHeight: window ? Math.max(1, window.Screen.desktopAvailableHeight) : 1
    readonly property int maximumWidth: Math.min(960, availableWidth)
    readonly property int maximumHeight: Math.min(800, availableHeight)
    readonly property int minimumWidth: Math.min(480, maximumWidth)
    readonly property int minimumHeight: Math.min(360, maximumHeight)
    readonly property size constrainedSize: Qt.size(
        Math.max(minimumWidth, Math.min(requestedSize.width, maximumWidth)),
        Math.max(minimumHeight, Math.min(requestedSize.height, maximumHeight)))
    readonly property size observedSize: window ? Qt.size(window.width, window.height) : Qt.size(0, 0)
    readonly property real scale: window ? window.Screen.devicePixelRatio : 1
    property string moveResult: "Not requested"
    property string resizeResult: "Not requested"

    function applySize() {
        if (!window)
            return
        window.width = constrainedSize.width
        window.height = constrainedSize.height
    }

    function attach(target) {
        window = target
        window.minimumWidth = Qt.binding(function() { return policy.minimumWidth })
        window.minimumHeight = Qt.binding(function() { return policy.minimumHeight })
        window.maximumWidth = Qt.binding(function() { return policy.maximumWidth })
        window.maximumHeight = Qt.binding(function() { return policy.maximumHeight })
        applySize()
        attached = true
    }

    function startMove() {
        moveResult = window.startSystemMove() ? "Accepted by Qt; observe compositor" : "Rejected by Qt"
    }

    function startResize() {
        resizeResult = window.startSystemResize(Qt.RightEdge | Qt.BottomEdge)
            ? "Accepted by Qt; observe compositor" : "Rejected by Qt"
    }

    onConstrainedSizeChanged: applySize()
    // Never save or restore x/y: Wayland placement belongs to the compositor.
}
