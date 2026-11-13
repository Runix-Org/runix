"""Shared native Wayland capture mechanics; verification tools only."""

import json
import socket
import subprocess
import time


def command(*args):
    try:
        return subprocess.check_output(args, text=True, stderr=subprocess.STDOUT, timeout=5).strip()
    except subprocess.CalledProcessError as error:
        raise RuntimeError(f"{args[0]} failed ({error.returncode}): {error.output.strip()}") from error


def ipc(path, op, **fields):
    identifier = str(time.monotonic_ns())
    message = dict(version=1, id=identifier, op=op,
                   deadline_ms=int(time.time() * 1000) + 1500, **fields)
    with socket.socket(socket.AF_UNIX) as connection:
        connection.settimeout(2)
        connection.connect(str(path))
        connection.sendall(json.dumps(message).encode() + b"\n")
        with connection.makefile("rb") as stream:
            line = stream.readline(65537)
    if not line.endswith(b"\n") or len(line) > 65536:
        raise RuntimeError("The debug response exceeds the frame limit")
    response = json.loads(line)
    if response.get("id") != identifier or response.get("version") != 1:
        raise RuntimeError("The debug response does not match the request")
    if response.get("error"):
        raise RuntimeError(op + " " + fields.get("target", "") + ": " + response["error"]["message"])
    return response["result"]


def wait_for(check, description):
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        result = check()
        if result:
            return result
        time.sleep(0.05)
    raise RuntimeError("Timed out waiting for " + description)


def write_json(path, value):
    path.write_text(json.dumps(value, indent=2) + "\n")


class NativeWindow:
    def __init__(self, path, process):
        self.path = path
        self.process = process

    def snapshot(self):
        if self.process.poll() is not None:
            raise RuntimeError("The owned application exited; inspect its log")
        return ipc(self.path, "snapshot")

    def controls(self):
        return {entry["id"]: entry for entry in self.snapshot()["controls"]}

    def action(self, target, action="activate", text=""):
        return ipc(self.path, "action", target=target, action=action, text=text)

    def client(self):
        clients = json.loads(command("hyprctl", "-j", "clients"))
        return next((entry for entry in clients if entry["pid"] == self.process.pid and entry["mapped"]), None)

    def ready(self):
        active = json.loads(command("hyprctl", "-j", "activewindow"))
        # Qt activity and compositor input ownership are separate observations.
        return self.client() and active.get("pid") == self.process.pid

    def focused(self):
        active = json.loads(command("hyprctl", "-j", "activewindow"))
        if active.get("pid") != self.process.pid:
            raise RuntimeError("Capture stopped: another window has input focus")

    def key(self, key, modifiers=""):
        self.focused()
        selector = "pid:" + str(self.process.pid)
        command("hyprctl", "dispatch", "hl.dsp.send_shortcut({window=" + json.dumps(selector)
                + ",mods=" + json.dumps(modifiers) + ",key=" + json.dumps(key) + "})")

    def point_at(self, pointer, monitors, x, y):
        self.focused()
        client = self.client()
        if not client:
            raise RuntimeError("The owned window is not mapped")
        window = self.snapshot()["window"]
        if not (0 <= x < window["width"] and 0 <= y < window["height"]):
            raise RuntimeError("The pointer target is outside the owned window")
        x *= client["size"][0] / window["width"]
        y *= client["size"][1] / window["height"]
        left = min(m["x"] for m in monitors)
        top = min(m["y"] for m in monitors)
        right = max(m["x"] + m["width"] / m["scale"] for m in monitors)
        bottom = max(m["y"] + m["height"] / m["scale"] for m in monitors)
        pointer.move(round(client["at"][0] + x - left), round(client["at"][1] + y - top),
                     round(right - left), round(bottom - top))

    def point(self, pointer, monitors, target=None):
        if target:
            control = self.controls()[target]
            if not control["in_view"] or control["clipped"]:
                raise RuntimeError("The target is not fully visible: " + target)
            bounds = control["bounds"]
            x = bounds["x"] + bounds["width"] / 2
            y = bounds["y"] + bounds["height"] / 2
        else:
            x, y = 10, 10
        self.point_at(pointer, monitors, x, y)
