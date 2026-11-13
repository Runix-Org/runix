import QtQuick
import QtQuick.Window

QtObject {
    id: policy
    property bool attached: false
    property Window window
    property bool expanded: false
    // An enabled compositor backend clamps restored bounds in its own coordinate space.
    property bool compositorPlacement: false
    property size savedSize: Qt.size(0, 0)
    readonly property size requestedSize: savedSize.width > 0 && savedSize.height > 0
        ? savedSize : expanded ? Qt.size(800, 640) : Qt.size(640, 420)
    readonly property int availableWidth: window ? Math.max(1, window.Screen.desktopAvailableWidth) : 1
    readonly property int availableHeight: window ? Math.max(1, window.Screen.desktopAvailableHeight) : 1
    readonly property int minimumWidth: Math.min(480, availableWidth)
    readonly property int minimumHeight: Math.min(360, availableHeight)
    readonly property size constrainedSize: Qt.size(
        Math.max(minimumWidth, compositorPlacement ? requestedSize.width : Math.min(requestedSize.width, availableWidth)),
        Math.max(minimumHeight, compositorPlacement ? requestedSize.height : Math.min(requestedSize.height, availableHeight)))
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
        window.objectName = "runix-policy-window"
        window.minimumWidth = Qt.binding(function() { return policy.minimumWidth })
        window.minimumHeight = Qt.binding(function() { return policy.minimumHeight })
        applySize()
        attached = true
    }

    function startMove() {
        moveResult = window.startSystemMove() ? "Qt accepted the request. Check the compositor." : "Qt rejected the request."
    }

    function startResize() {
        resizeResult = window.startSystemResize(Qt.RightEdge | Qt.BottomEdge)
            ? "Qt accepted the request. Check the compositor." : "Qt rejected the request."
    }

    onConstrainedSizeChanged: applySize()
    // Product placement is handled by its optional compositor integration, not Qt x/y.
}
