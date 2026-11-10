"""Minimal client for wlr-virtual-pointer-unstable-v1 (Wayland)."""

import os
import socket
import struct
import time


_SOCKET_TIMEOUT = 2.0
_BUTTON_LEFT = 0x110
_PRESSED = 1
_RELEASED = 0


class Pointer:
    """Create and drive one compositor virtual pointer.

    move() uses output-layout coordinates: x/y are positions, width/height are
    the complete layout extents, as required by motion_absolute.
    """

    def __init__(self):
        self._sock = None
        self._globals = {}
        self._next_id = 3  # 1 is wl_display, 2 is wl_registry.
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
        display = os.environ.get("WAYLAND_DISPLAY")
        runtime = os.environ.get("XDG_RUNTIME_DIR")
        if not display or not runtime:
            raise RuntimeError("Wayland display environment is unavailable")
        path = display if os.path.isabs(display) else os.path.join(runtime, display)
        sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        sock.settimeout(_SOCKET_TIMEOUT)
        try:
            sock.connect(path)
            self._sock = sock

            self._send(1, 1, struct.pack("=I", 2))  # wl_display.get_registry(new id)
            self._roundtrip()
            found = [name for name, (interface, _version) in self._globals.items()
                     if interface == "zwlr_virtual_pointer_manager_v1"]
            if not found:
                raise RuntimeError("Compositor does not advertise zwlr_virtual_pointer_manager_v1")
            global_name = found[0]
            self._manager = self._next_id
            self._next_id += 1
            version = min(self._globals[global_name][1], 2)

            encoded = b"zwlr_virtual_pointer_manager_v1\0"
            padded = encoded + b"\0" * (-len(encoded) % 4)
            bind = struct.pack("=II", global_name, len(encoded)) + padded
            bind += struct.pack("=II", version, self._manager)
            self._send(2, 0, bind)  # wl_registry.bind
            self._pointer = self._next_id
            self._next_id += 1

            # create_virtual_pointer(nullable seat, new id)
            self._send(self._manager, 0, struct.pack("=II", 0, self._pointer))
            self._roundtrip()
        except (OSError, RuntimeError, struct.error):
            self.close()
            raise

    def _send(self, obj, opcode, payload=b""):
        if self._sock is None:
            raise RuntimeError("Virtual pointer is not connected")
        self._sock.sendall(struct.pack("=II", obj, (8 + len(payload)) << 16 | opcode) + payload)

    def _recv_exact(self, size):
        data = bytearray()
        while len(data) < size:
            chunk = self._sock.recv(size - len(data))
            if not chunk:
                raise RuntimeError("Wayland compositor disconnected")
            data.extend(chunk)
        return bytes(data)

    def _event(self):
        obj, word = struct.unpack("=II", self._recv_exact(8))
        size, opcode = word >> 16, word & 0xffff
        if size < 8:
            raise RuntimeError("Invalid Wayland event")
        return obj, opcode, self._recv_exact(size - 8)

    def _roundtrip(self):
        callback = self._next_id
        self._next_id += 1
        self._send(1, 0, struct.pack("=I", callback))  # wl_display.sync
        while True:
            obj, opcode, payload = self._event()
            if obj == 2 and opcode == 0:  # wl_registry.global
                name, = struct.unpack_from("=I", payload)
                length, = struct.unpack_from("=I", payload, 4)
                padded_length = (length + 3) & ~3
                interface = payload[8:8 + length - 1].decode("ascii")
                version, = struct.unpack_from("=I", payload, 8 + padded_length)
                self._globals[name] = (interface, version)
            elif obj == 2 and opcode == 1:  # wl_registry.global_remove
                name, = struct.unpack("=I", payload)
                self._globals.pop(name, None)
            elif obj == callback:
                return
            elif obj == 1 and opcode == 0:
                raise RuntimeError("Wayland compositor reported a protocol error")

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
