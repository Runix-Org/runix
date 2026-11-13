"""Verification-only virtual keyboard with a private US keymap, not session changes."""

import array
import ctypes
import ctypes.util
import os
import socket
import struct
import time

from wayland_input import Connection


# Linux evdev keycodes; the private US keymap fixes their meaning on this device.
LETTERS = dict(zip("qwertyuiop", range(16, 26)))
LETTERS.update(zip("asdfghjkl", range(30, 39)))
LETTERS.update(zip("zxcvbnm", range(44, 51)))


def us_keymap():
    library = ctypes.util.find_library("xkbcommon")
    if not library:
        raise RuntimeError("Virtual keyboard needs libxkbcommon")
    xkb = ctypes.CDLL(library)
    class Names(ctypes.Structure):
        _fields_ = [(name, ctypes.c_char_p) for name in ("rules", "model", "layout", "variant", "options")]
    xkb.xkb_context_new.argtypes = [ctypes.c_int]
    xkb.xkb_context_new.restype = ctypes.c_void_p
    xkb.xkb_keymap_new_from_names.argtypes = [ctypes.c_void_p, ctypes.POINTER(Names), ctypes.c_int]
    xkb.xkb_keymap_new_from_names.restype = ctypes.c_void_p
    xkb.xkb_keymap_get_as_string.argtypes = [ctypes.c_void_p, ctypes.c_int]
    xkb.xkb_keymap_get_as_string.restype = ctypes.c_void_p
    xkb.xkb_keymap_unref.argtypes = [ctypes.c_void_p]
    xkb.xkb_context_unref.argtypes = [ctypes.c_void_p]
    libc = ctypes.CDLL(None)
    libc.free.argtypes = [ctypes.c_void_p]
    context = xkb.xkb_context_new(0)
    keymap = text = None
    try:
        if not context:
            raise RuntimeError("Could not create a keyboard context")
        names = Names(b"evdev", b"pc105", b"us", b"", b"")
        keymap = xkb.xkb_keymap_new_from_names(context, ctypes.byref(names), 0)
        if not keymap:
            raise RuntimeError("Could not create the private US keymap")
        text = xkb.xkb_keymap_get_as_string(keymap, 1)
        if not text:
            raise RuntimeError("Could not serialize the private US keymap")
        return ctypes.string_at(text) + b"\0"
    finally:
        if text:
            libc.free(text)
        if keymap:
            xkb.xkb_keymap_unref(keymap)
        if context:
            xkb.xkb_context_unref(context)


class Keyboard(Connection):
    def __init__(self):
        super().__init__()
        self.keyboard = None
        self.held = None

    def __enter__(self):
        try:
            self.connect()
            manager = self.bind("zwp_virtual_keyboard_manager_v1")
            seat = self.bind("wl_seat")
            self.keyboard = self.allocate()
            self._send(manager, 0, struct.pack("=II", seat, self.keyboard))
            keymap = us_keymap()
            fd = os.memfd_create("runix-private-keymap", os.MFD_CLOEXEC)
            try:
                with os.fdopen(fd, "wb", closefd=False) as stream:
                    stream.write(keymap)
                payload = struct.pack("=II", 1, len(keymap))
                packet = struct.pack("=II", self.keyboard, (8 + len(payload)) << 16) + payload
                sent = self._sock.sendmsg([packet], [(socket.SOL_SOCKET, socket.SCM_RIGHTS, array.array("i", [fd]))])
                if sent != len(packet):
                    raise RuntimeError("Could not send the complete keyboard keymap")
            finally:
                os.close(fd)
            self._roundtrip()
            return self
        except BaseException:
            self.close()
            raise

    def press(self, key, ctrl=False):
        code = 28 if key == "Return" else LETTERS[key]
        self._send(self.keyboard, 2, struct.pack("=IIII", 4 if ctrl else 0, 0, 0, 0))
        self.held = code
        self._send(self.keyboard, 1, struct.pack("=III", self._time(), code, 1))
        self._send(self.keyboard, 1, struct.pack("=III", self._time(), code, 0))
        self.held = None
        self._send(self.keyboard, 2, struct.pack("=IIII", 0, 0, 0, 0))
        self._roundtrip()

    @staticmethod
    def _time():
        return int(time.monotonic() * 1000) & 0xffffffff

    def close(self):
        try:
            if self.keyboard is not None and self._sock is not None:
                if self.held is not None:
                    self._send(self.keyboard, 1, struct.pack("=III", self._time(), self.held, 0))
                self._send(self.keyboard, 2, struct.pack("=IIII", 0, 0, 0, 0))
                self._send(self.keyboard, 3)
                self._roundtrip()
        finally:
            self.keyboard = None
            super().close()
