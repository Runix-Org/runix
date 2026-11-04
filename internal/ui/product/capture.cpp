#include "capture.h"

#include <QtGui/QGuiApplication>
#include <QtGui/QWindow>
#include <QtGui/qguiapplication_platform.h>
#include <wayland-client.h>
#include <cstring>

extern "C" void runixCaptureChanged(uintptr_t handle, int active);

namespace {
// keyboard-shortcuts-inhibit-unstable-v1, version 1. These are the complete
// manager/inhibitor wire descriptions; no generated Qt plugin or private ABI.
const wl_message inhibitorRequests[] = {{"destroy", "", nullptr}};
const wl_message inhibitorEvents[] = {{"active", "", nullptr}, {"inactive", "", nullptr}};
const wl_interface inhibitorInterface = {
    "zwp_keyboard_shortcuts_inhibitor_v1", 1, 1, inhibitorRequests, 2, inhibitorEvents
};
const wl_interface *inhibitTypes[] = {&inhibitorInterface, &wl_surface_interface, &wl_seat_interface};
const wl_message managerRequests[] = {{"destroy", "", nullptr}, {"inhibit_shortcuts", "noo", inhibitTypes}};
const wl_interface managerInterface = {
    "zwp_keyboard_shortcuts_inhibit_manager_v1", 1, 2, managerRequests, 0, nullptr
};

struct Capture {
    uintptr_t handle;
    wl_registry *registry = nullptr;
    wl_proxy *manager = nullptr;
    wl_proxy *inhibitor = nullptr;
    uint32_t managerName = 0;
};

void stop(Capture *capture) {
    if (capture->inhibitor) {
        wl_proxy_marshal_flags(capture->inhibitor, 0, nullptr, 1, WL_MARSHAL_FLAG_DESTROY);
        capture->inhibitor = nullptr;
    }
}

void global(void *data, wl_registry *registry, uint32_t name, const char *interface, uint32_t) {
    auto *capture = static_cast<Capture *>(data);
    if (!capture->manager && std::strcmp(interface, managerInterface.name) == 0) {
        capture->manager = static_cast<wl_proxy *>(wl_registry_bind(registry, name, &managerInterface, 1));
        capture->managerName = name;
    }
}

void globalRemove(void *data, wl_registry *, uint32_t name) {
    auto *capture = static_cast<Capture *>(data);
    if (capture->managerName == name && capture->manager) {
        stop(capture);
        wl_proxy_marshal_flags(capture->manager, 0, nullptr, 1, WL_MARSHAL_FLAG_DESTROY);
        capture->manager = nullptr;
        capture->managerName = 0;
        runixCaptureChanged(capture->handle, 0);
    }
}

void active(void *data, wl_proxy *) {
    runixCaptureChanged(static_cast<Capture *>(data)->handle, 1);
}

void inactive(void *data, wl_proxy *) {
    runixCaptureChanged(static_cast<Capture *>(data)->handle, 0);
}

const wl_registry_listener registryListener = {global, globalRemove};
struct InhibitorListener {
    void (*active)(void *, wl_proxy *);
    void (*inactive)(void *, wl_proxy *);
};
const InhibitorListener inhibitorListener = {active, inactive};
} // namespace

void *runix_capture_new(uintptr_t handle) {
    auto *capture = new Capture{handle};
    auto *native = qGuiApp->nativeInterface<QNativeInterface::QWaylandApplication>();
    if (native) {
        capture->registry = wl_display_get_registry(native->display());
        wl_registry_add_listener(capture->registry, &registryListener, capture);
    }
    return capture;
}

int runix_capture_start(void *value) {
    auto *capture = static_cast<Capture *>(value);
    auto *native = qGuiApp->nativeInterface<QNativeInterface::QWaylandApplication>();
    QWindow *window = QGuiApplication::focusWindow();
    if (!capture->manager || !native || !window || !window->isVisible())
        return 0;
    wl_seat *seat = native->lastInputSeat();
    if (!seat)
        seat = native->seat();
    if (!seat)
        return 0;
    stop(capture);
    // Qt's Wayland QPlatformWindow::winId() is the wl_surface, not an X11 ID.
    auto *surface = reinterpret_cast<wl_surface *>(window->winId());
    if (!surface)
        return 0;
    capture->inhibitor = wl_proxy_marshal_flags(capture->manager, 1, &inhibitorInterface, 1, 0,
                                               nullptr, surface, seat);
    if (!capture->inhibitor)
        return 0;
    wl_proxy_add_listener(capture->inhibitor,
                         reinterpret_cast<void (**)(void)>(const_cast<InhibitorListener *>(&inhibitorListener)),
                         capture);
    wl_display_flush(native->display());
    return 1;
}

void runix_capture_stop(void *value) {
    stop(static_cast<Capture *>(value));
}

void runix_capture_delete(void *value) {
    auto *capture = static_cast<Capture *>(value);
    stop(capture);
    if (capture->manager)
        wl_proxy_marshal_flags(capture->manager, 0, nullptr, 1, WL_MARSHAL_FLAG_DESTROY);
    if (capture->registry)
        wl_registry_destroy(capture->registry);
    delete capture;
}
