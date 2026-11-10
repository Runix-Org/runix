#!/usr/bin/env python3
"""Capture real runix laboratory controls on native Hyprland Wayland."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time

sys.dont_write_bytecode = True

from wayland_pointer import Pointer


THEMES = {
    "sonoma-dark": ("ikupenov/sonoma-dark.json", "dark"),
    "sonoma-light": ("ikupenov/sonoma-light.json", "light"),
    "brutalist-dark": ("degouville/brutalist-dark.json", "dark"),
    "brutalist-light": ("degouville/brutalist-light.json", "light"),
    "mocha": ("catppuccin-org/catppuccin-mocha.json", "dark"),
    "latte": ("catppuccin-org/catppuccin-latte.json", "light"),
    "solarised": ("daniel/gradient-solarised.json", "light"),
    "alpenglow": ("dannyseidel/alpenglow.json", "dark"),
    "low-contrast": (None, "dark"),
    "impossible-gradient": (None, "dark"),
}
STATES = {
    "default": "gallery-search", "hover": "gallery-action", "pressed": "gallery-action",
    "focus": "gallery-action", "disabled": "gallery-disabled", "readonly": "gallery-readonly",
    "checked": "gallery-checked", "error": "gallery-error", "busy": "gallery-busy",
    "popup": "gallery-selection", "menu": "gallery-menu", "dialog": "gallery-dialog",
    "popover": "gallery-popover", "tooltip": "gallery-tooltip", "notes": "gallery-notes",
    "radio": "gallery-radio-selected", "switch": "gallery-switch-on", "slider": "gallery-slider",
    "status": "gallery-status", "loading": "gallery-loading", "empty": "gallery-empty",
    "recorder": "gallery-recorder", "tags": "gallery-tag",
    "diagnostics": "gallery-inspection",
    "selected": None, "virtualized": None, "table": None, "form": None, "geometry": None,
}
REVISION = "1298a944166d53d59e6211609366cbfdd08da623"


def command(*args):
    return subprocess.check_output(args, text=True, timeout=5).strip()


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


def fixture(config, name):
    colors = dict.fromkeys(("background", "backgroundSecondary", "text", "selection", "loader",
                           "red", "orange", "yellow", "green", "blue", "purple", "magenta"), "#202020")
    if name == "impossible-gradient":
        colors.update(background="#000000", backgroundSecondary="#FFFFFF")
    value = dict(version=1, name="Private " + name, appearance="dark", colors=colors)
    data = json.dumps(value).encode()
    digest = hashlib.sha256(data).hexdigest()
    directory = config / "runix/themes/imports"
    directory.mkdir(parents=True)
    (directory / (digest + ".json")).write_bytes(data)
    return "imports/" + digest + ".json"


class Lab:
    def __init__(self, path, process):
        self.path = path
        self.process = process

    def snapshot(self):
        if self.process.poll() is not None:
            raise RuntimeError("The laboratory exited; inspect lab.log")
        return ipc(self.path, "snapshot")

    def controls(self):
        return {entry["id"]: entry for entry in self.snapshot()["controls"]}

    def action(self, target, action="activate", text=""):
        return ipc(self.path, "action", target=target, action=action, text=text)

    def client(self):
        clients = json.loads(command("hyprctl", "-j", "clients"))
        return next((entry for entry in clients if entry["pid"] == self.process.pid and entry["mapped"]), None)

    def geometry_ready(self):
        client = self.client()
        snapshot = self.snapshot()
        status = next(entry["model"] for entry in snapshot["controls"] if entry["id"] == "capture-status")
        if status["resize_status"].startswith("Hyprland resize failed:"):
            raise RuntimeError(status["resize_status"])
        signature = (tuple(client["at"]), tuple(client["size"]), snapshot["window"]["width"],
                     snapshot["window"]["height"], status["resize_status"])
        now = time.monotonic()
        if signature != getattr(self, "_geometry", None):
            self._geometry, self._geometry_since = signature, now
        return (status["resize_status"].startswith("Hyprland command accepted")
                and status["frame_swapped"] and now - self._geometry_since >= 0.3)

    def ready(self):
        active = json.loads(command("hyprctl", "-j", "activewindow"))
        # Qt's active flag is not compositor input ownership; retain it in metadata.
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

    def frame(self):
        controls = self.controls()
        previous = controls["capture-status"]["model"]["frames"]
        self.action("capture-status", "activate")
        return wait_for(lambda: self.snapshot_after(previous), "a rendered frame")

    def snapshot_after(self, previous):
        snapshot = self.snapshot()
        status = next(entry for entry in snapshot["controls"] if entry["id"] == "capture-status")
        return snapshot if status["model"]["frames"] > previous else None

    def point(self, pointer, monitors, target=None):
        self.focused()
        client = self.client()
        if target:
            control = self.controls()[target]
            if not control["in_view"] or control["clipped"]:
                raise RuntimeError("The target is not fully visible: " + target)
            bounds = control["bounds"]
            x = bounds["x"] + bounds["width"] / 2
            y = bounds["y"] + bounds["height"] / 2
        else:
            x, y = 10, 10
        window = self.snapshot()["window"]
        x *= client["size"][0] / window["width"]
        y *= client["size"][1] / window["height"]
        # zwlr_virtual_pointer absolute motion is relative to the complete layout.
        left = min(m["x"] for m in monitors)
        top = min(m["y"] for m in monitors)
        right = max(m["x"] + m["width"] / m["scale"] for m in monitors)
        bottom = max(m["y"] + m["height"] / m["scale"] for m in monitors)
        pointer.move(round(client["at"][0] + x - left), round(client["at"][1] + y - top),
                     round(right - left), round(bottom - top))


def prepare(lab, state, pointer, monitors):
    target = STATES[state]
    lab.action("gallery-mode")
    lab.frame()
    lab.point(pointer, monitors)
    if target:
        lab.action("gallery-scroll", "scroll", target)
        lab.frame()
        if state in ("hover", "pressed", "popup", "menu", "dialog", "popover", "tooltip"):
            lab.point(pointer, monitors, target)
            if state != "hover":
                lab.focused()
                pointer.button(True)
                if state != "pressed":
                    pointer.button(False)
        elif state in ("focus", "error"):
            lab.action(target, "focus")
            lab.key("Tab")
            lab.key("Tab", "SHIFT")
        elif state == "readonly":
            lab.action(target, "focus")
            lab.action(target, "select")
    elif state in ("selected", "virtualized"):
        lab.action("search-mode")
        wait_for(lambda: not lab.controls()["results"]["model"]["state"]["loading"], "search results")
        lab.action("results", "select", "150" if state == "virtualized" else "2")
        lab.action("query", "focus")
        lab.key("Down")
        lab.point(pointer, monitors)
    elif state == "table":
        lab.action("table-mode")
        lab.frame()
        lab.action("table", "select", "9000")
        lab.action("table", "focus")
        lab.key("Down")
    elif state == "form":
        lab.action("form-mode")
        lab.action("form-name", "focus")
        lab.key("Return")
    elif state == "geometry":
        lab.action("geometry-open")
    return target


def capture(args):
    if os.environ.get("QT_QPA_PLATFORM") == "offscreen" or not os.environ.get("WAYLAND_DISPLAY"):
        raise RuntimeError("Capture needs a native Wayland session")
    output = Path(args.output).resolve()
    output.mkdir(mode=0o700)  # Never overwrite an earlier evidence directory.
    binary = Path(args.lab).resolve(strict=True)
    monitors = json.loads(command("hyprctl", "-j", "monitors"))
    if len(monitors) != 1 or monitors[0]["transform"] != 0:
        raise RuntimeError("This capture recipe supports one unrotated output; do not change desktop settings")
    compositor = json.loads(command("hyprctl", "-j", "version"))
    with tempfile.TemporaryDirectory(prefix="runix-capture-") as temporary:
        root = Path(temporary)
        env = os.environ.copy()
        for name in ("CONFIG", "DATA", "STATE", "CACHE"):
            directory = root / name.lower()
            directory.mkdir()
            env["XDG_" + name + "_HOME"] = str(directory)
        empty = root / "empty"
        empty.mkdir()
        env.update(XDG_DATA_DIRS=str(empty), QT_QPA_PLATFORM="wayland", QT_QUICK_BACKEND="software",
                   QT_SCALE_FACTOR="1")
        env.pop("QT_SCREEN_SCALE_FACTORS", None)
        config = root / "config"
        (config / "runix").mkdir()
        identity, appearance = THEMES[args.theme]
        if identity is None:
            identity = fixture(config, args.theme)
        preferences = dict(mode=appearance, light_id="ikupenov/sonoma-light.json",
                           dark_id="ikupenov/sonoma-dark.json", improve_contrast=args.correction == "on")
        preferences[appearance + "_id"] = identity
        write_json(config / "runix/config.json", dict(appearance=preferences))
        endpoint = root / "debug.sock"
        with (output / "lab.log").open("w") as log:
            process = subprocess.Popen([str(binary), "--hyprland-window", "--debug-socket", str(endpoint),
                                        "--activation-socket", str(root / "activation.sock")],
                                       cwd=empty, env=env, stdout=log, stderr=log)
            try:
                lab = Lab(endpoint, process)
                wait_for(lambda: endpoint.exists() and lab.client(), "the mapped native laboratory window")
                wait_for(lab.geometry_ready, "settled native window geometry")
                command("hyprctl", "dispatch", "hl.dsp.focus({window="
                        + json.dumps("pid:" + str(process.pid)) + "})")
                wait_for(lambda: endpoint.exists() and lab.ready(), "the focused native laboratory window")
                lab.frame()
                status = lab.controls()["appearance-status"]["model"]
                if status["active_id"] != identity or status["error"]:
                    raise RuntimeError("The requested theme was not applied")
                with Pointer() as pointer:
                    target = prepare(lab, args.state, pointer, monitors)
                    before = lab.controls()
                    if args.switch_theme:
                        identity, appearance = THEMES[args.switch_theme]
                        lab.action("appearance-" + appearance, "select", identity)
                        lab.action("appearance-mode", "select", appearance)
                    if args.toggle_correction:
                        lab.action("appearance-contrast")
                    # Allow native popup transitions/tooltips to settle, then require a fresh frame.
                    time.sleep(0.7)
                    snapshot = lab.frame()
                    lab.focused()
                    controls = {entry["id"]: entry for entry in snapshot["controls"]}
                    effective = controls["appearance-status"]["model"]
                    if effective["active_id"] != identity or effective["error"]:
                        raise RuntimeError("The live appearance choice was not applied")
                    if args.state in ("selected", "virtualized", "table"):
                        owner = "table" if args.state == "table" else "results"
                        if before[owner]["model"]["state"]["selected_id"] != controls[owner]["model"]["state"]["selected_id"]:
                            raise RuntimeError("The selected record changed during capture")
                        expected_index = {"selected": 3, "virtualized": 151, "table": 9001}[args.state]
                        if controls[owner]["model"]["state"]["selected_index"] != expected_index:
                            raise RuntimeError("Native keyboard input did not move the selection")
                        if owner == "results":
                            rows = controls[owner]["model"]["visible_rows"]
                            if not any(row["selected"] and not row["hovered"] for row in rows):
                                raise RuntimeError("The persistent selection is not visible away from the pointer")
                        elif not any(cell["row"] == expected_index for cell in controls[owner]["model"]["visible_cells"]):
                            raise RuntimeError("The selected table row is outside the viewport")
                    if args.state in ("hover", "pressed", "focus", "error"):
                        model = controls[target]["model"]
                        required = "pressed" if args.state == "pressed" else "hovered" if args.state == "hover" else "focused"
                        if not model[required]:
                            raise RuntimeError("Real input did not reach the requested state: " + required)
                    if args.state in ("popup", "menu", "dialog", "popover", "tooltip"):
                        if not controls[target]["model"]["popup_open"]:
                            raise RuntimeError("Real input did not open the requested overlay")
                    if args.state == "focus" and not controls[target]["model"]["visual_focus"]:
                        raise RuntimeError("Keyboard traversal did not show the focus ring")
                    if args.state == "disabled" and controls[target]["enabled"]:
                        raise RuntimeError("The disabled specimen is enabled")
                    if args.state == "readonly" and not controls[target]["model"]["read_only"]:
                        raise RuntimeError("The read-only specimen is editable")
                    if args.state in ("checked", "busy") and not controls[target]["model"][args.state]:
                        raise RuntimeError("The specimen did not reach the requested state")
                    if args.state == "geometry" and not controls["geometry-popup"]["model"]["opened"]:
                        raise RuntimeError("The geometry popup did not open")
                    if args.state == "form" and not controls["form-status"]["model"]["errors"]:
                        raise RuntimeError("Native submission did not show validation errors")
                    client = lab.client()
                    x, y = client["at"]
                    width, height = client["size"]
                    image = output / "native.png"
                    command("grim", "-g", f"{x},{y} {width}x{height}", str(image))
                    write_json(output / "snapshot.json", snapshot)
                    write_json(output / "capture.json", dict(
                        format=1, theme=identity, upstream_revision=REVISION if identity[0:8] != "imports/" else None,
                        fixture=args.theme if identity.startswith("imports/") else None,
                        state=args.state, requested_theme=args.theme, switch_theme=args.switch_theme,
                        correction=effective["preferences"]["improve_contrast"],
                        toggle_correction=args.toggle_correction, screenshot=str(image),
                        evidence="native-wayland-real-input" if args.state in
                        ("hover", "pressed", "focus", "error", "popup", "menu", "dialog", "popover", "tooltip", "selected", "virtualized", "table")
                        else "native-wayland-semantic-fixture-supplementary",
                        renderer=controls["capture-status"]["model"], window=snapshot["window"],
                        appearance=controls["appearance-status"]["model"],
                        compositor={key: compositor.get(key) for key in ("version", "commit")},
                        output={key: monitors[0][key] for key in ("name", "scale", "width", "height", "transform")},
                        compositor_bounds={"at": client["at"], "size": client["size"]},
                        binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest()))
            finally:
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
    print(output / "capture.json")


if __name__ == "__main__":
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--lab", required=True, help="Absolute path to the built ui-lab binary")
    parser.add_argument("--output", required=True, help="New private artifact directory; must not exist")
    parser.add_argument("--theme", choices=THEMES, default="sonoma-dark")
    parser.add_argument("--correction", choices=("off", "on"), default="off")
    parser.add_argument("--state", choices=STATES, default="default")
    parser.add_argument("--switch-theme", choices=[key for key, value in THEMES.items() if value[0]],
                        help="Change the theme live after preparing the state")
    parser.add_argument("--toggle-correction", action="store_true", help="Toggle correction with the state active")
    arguments = parser.parse_args()
    try:
        capture(arguments)
    except (OSError, RuntimeError, ValueError, KeyError, StopIteration, subprocess.SubprocessError) as error:
        parser.exit(1, f"Capture failed: {error}\n")
