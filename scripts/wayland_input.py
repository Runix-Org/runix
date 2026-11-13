"""Small bounded Wayland wire transport for verification-only virtual input."""

import os
import socket
import struct


class Connection:
    def __init__(self):
        self._sock = None
        self._globals = {}
        self._next_id = 3

    def connect(self):
        display = os.environ.get("WAYLAND_DISPLAY")
        runtime = os.environ.get("XDG_RUNTIME_DIR")
        if not display or not runtime:
            raise RuntimeError("Wayland display environment is unavailable")
        path = display if os.path.isabs(display) else os.path.join(runtime, display)
        self._sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self._sock.settimeout(2)
        self._sock.connect(path)
        self._send(1, 1, struct.pack("=I", 2))
        self._roundtrip()

    def allocate(self):
        identifier = self._next_id
        self._next_id += 1
        return identifier

    def bind(self, interface, maximum=1):
        found = [(name, version) for name, (kind, version) in self._globals.items() if kind == interface]
        if not found:
            raise RuntimeError("Compositor does not advertise " + interface)
        name, version = found[0]
        identifier = self.allocate()
        encoded = interface.encode("ascii") + b"\0"
        payload = struct.pack("=II", name, len(encoded)) + encoded + b"\0" * (-len(encoded) % 4)
        self._send(2, 0, payload + struct.pack("=II", min(version, maximum), identifier))
        return identifier

    def _send(self, obj, opcode, payload=b""):
        if self._sock is None:
            raise RuntimeError("Virtual input is not connected")
        self._sock.sendall(struct.pack("=II", obj, (8 + len(payload)) << 16 | opcode) + payload)

    def _recv_exact(self, size):
        data = bytearray()
        while len(data) < size:
            chunk = self._sock.recv(size - len(data))
            if not chunk:
                raise RuntimeError("Wayland compositor disconnected")
            data.extend(chunk)
        return bytes(data)

    def _roundtrip(self):
        callback = self.allocate()
        self._send(1, 0, struct.pack("=I", callback))
        while True:
            obj, word = struct.unpack("=II", self._recv_exact(8))
            size, opcode = word >> 16, word & 0xffff
            if size < 8 or size > 65535:
                raise RuntimeError("Invalid Wayland event")
            payload = self._recv_exact(size - 8)
            if obj == 2 and opcode == 0:
                name, length = struct.unpack_from("=II", payload)
                padded = (length + 3) & ~3
                interface = payload[8:8 + length - 1].decode("ascii")
                version, = struct.unpack_from("=I", payload, 8 + padded)
                self._globals[name] = (interface, version)
            elif obj == 2 and opcode == 1:
                name, = struct.unpack("=I", payload)
                self._globals.pop(name, None)
            elif obj == callback:
                return
            elif obj == 1 and opcode == 0:
                raise RuntimeError("Wayland compositor reported a protocol error")

    def close(self):
        if self._sock is not None:
            self._sock.close()
            self._sock = None
