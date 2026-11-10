#include "logging.h"

#include <QtCore/QLoggingCategory>
#include <QtCore/QStringView>
#include <cstring>

namespace {
// Keep this diagnostic opt-in without disabling any Qt logging category.
Q_LOGGING_CATEGORY(focusDiagnostics, "runix.qt.textinput", QtWarningMsg)
QtMessageHandler previousHandler = nullptr;

bool repeatedSurfaceFocus(QStringView message) {
    constexpr QLatin1StringView prefix(
        "virtual void QtWaylandClient::QWaylandTextInputv3::enableSurface(wl_surface*) "
        "Trying to enable ");
    constexpr QLatin1StringView separator(" but ");
    constexpr QLatin1StringView suffix(" has focus");
    if (!message.startsWith(prefix) || !message.endsWith(suffix))
        return false;

    message = message.sliced(prefix.size(), message.size() - prefix.size() - suffix.size());
    const auto split = message.indexOf(separator);
    if (split < 0)
        return false;
    const auto surface = message.first(split);
    if (!surface.startsWith(QLatin1StringView("0x")) || surface.size() <= 2
        || surface != message.sliced(split + separator.size()))
        return false;

    bool nonzero = false;
    for (const QChar digit : surface.sliced(2)) {
        const auto value = digit.unicode();
        if (!((value >= '0' && value <= '9') || (value >= 'a' && value <= 'f')
              || (value >= 'A' && value <= 'F')))
            return false;
        nonzero = nonzero || value != '0';
    }
    return nonzero;
}

void handleMessage(QtMsgType type, const QMessageLogContext &context, const QString &message) {
    if (type == QtWarningMsg && context.category
        && std::strcmp(context.category, "qt.qpa.wayland.textinput") == 0
        && repeatedSurfaceFocus(message)) {
        if (focusDiagnostics().isDebugEnabled()) {
            const QMessageLogContext debugContext(context.file, context.line, context.function,
                                                  focusDiagnostics().categoryName());
            previousHandler(QtDebugMsg, debugContext, message);
        }
        return;
    }
    previousHandler(type, context, message);
}
} // namespace

// Product startup/shutdown owns this process-wide handler. Install before Qt
// creates threads; restore only after the application and its workers are gone.
void runix_logging_install(void) {
    previousHandler = qInstallMessageHandler(handleMessage);
}

void runix_logging_restore(void) {
    qInstallMessageHandler(previousHandler);
}
