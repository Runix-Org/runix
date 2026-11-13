"""Minimal client for wlr-virtual-pointer-unstable-v1 (Wayland)."""

import struct
import time

from wayland_input import Connection


_SOCKET_TIMEOUT = 2.0
_BUTTON_LEFT = 0x110
_PRESSED = 1
_RELEASED = 0


class Pointer(Connection):
    """Create and drive one compositor virtual pointer.

    move() uses output-layout coordinates: x/y are positions, width/height are
    the complete layout extents, as required by motion_absolute.
    """

    def __init__(self):
        super().__init__()
        self._held = False
        self._closed = False
        self._manager = None
        self._pointer = None

    def __enter__(self):
        self._connect()
        return self

    def __exit__(self, exc_type, exc, tb):
        self.close()
        return False

    def _connect(self):
        try:
            self.connect()
            self._manager = self.bind("zwlr_virtual_pointer_manager_v1", 2)
            self._pointer = self.allocate()
            self._send(self._manager, 0, struct.pack("=II", 0, self._pointer))
            self._roundtrip()
        except (OSError, RuntimeError, struct.error):
            self.close()
            raise


    def move(self, x, y, width, height):
        """Move to absolute compositor-layout coordinates."""
        if self._pointer is None or self._closed:
            raise RuntimeError("Virtual pointer is closed")
        values = (x, y, width, height)
        if any(not isinstance(value, int) or value < 0 or value > 0xffffffff for value in values):
            raise ValueError("Pointer coordinates and extents must be unsigned 32-bit integers")
        if width == 0 or height == 0 or x > width or y > height:
            raise ValueError("Pointer position must lie within nonzero layout extents")
        payload = struct.pack("=IIIII", self._time(), x, y, width, height)
        self._send(self._pointer, 1, payload)  # motion_absolute
        self._send(self._pointer, 4)  # frame

    def button(self, pressed):
        """Press or release the left mouse button."""
        if self._pointer is None or self._closed:
            raise RuntimeError("Virtual pointer is closed")
        if not isinstance(pressed, bool):
            raise TypeError("pressed must be a bool")
        if pressed == self._held:
            return
        self._send(self._pointer, 2, struct.pack("=III", self._time(), _BUTTON_LEFT,
                                                  _PRESSED if pressed else _RELEASED))
        self._send(self._pointer, 4)
        self._held = pressed

    @staticmethod
    def _time():
        return int(time.monotonic() * 1000) & 0xffffffff

    def close(self):
        if self._closed:
            return
        self._closed = True
        sock = self._sock
        if sock is None:
            return
        try:
            sock.settimeout(_SOCKET_TIMEOUT)
            if self._held and self._pointer is not None:
                self._send(self._pointer, 2, struct.pack("=III", self._time(), _BUTTON_LEFT, _RELEASED))
                self._send(self._pointer, 4)
                self._held = False
            if self._pointer is not None:
                self._send(self._pointer, 5)
            if self._manager is not None:
                self._send(self._manager, 1)
        except OSError:
            pass
        finally:
            self._sock = None
            sock.close()
