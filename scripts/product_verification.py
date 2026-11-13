"""Isolated product sessions for native scenarios and short exploratory checks."""

import base64
import hashlib
import os
from pathlib import Path
import signal
import subprocess
import tempfile
import time

from native_capture import NativeWindow, command, wait_for, write_json
from wayland_keyboard import Keyboard
from wayland_pointer import Pointer


LIMITS = ["Software Qt Quick on native Wayland; not GPU evidence.",
          "One unrotated output only; no multi-monitor, IME or global shortcut coverage.",
          "Semantic setup and frame delivery do not establish input behavior or visual approval.",
          "Screenshots need a separate visual assessment; no model is invoked by this runner."]
GEOMETRIES = {"normal": (640, 420), "constrained": (520, 360)}
FIXTURE_ID = "ZGVza3RvcA." + base64.urlsafe_b64encode(b"nativeproof-alpha.desktop").decode().rstrip("=")


class ProductWindow(NativeWindow):
    def frame(self):
        previous = self.snapshot()["rendering"]["frames"]
        self.action("render-frame")
        def delivered():
            snapshot = self.snapshot()
            return snapshot if snapshot["rendering"]["frames"] > previous else None
        return wait_for(delivered, "a new rendered product frame")

    def search_ready(self, query, count):
        self.focused()
        snapshot = self.snapshot()
        state = snapshot["search"]["state"]
        return snapshot if (snapshot["ready"] and snapshot["screen"] == "search"
                            and state["query"] == query and not state["loading"]
                            and state["generation"] == state["applied_generation"]
                            and state["count"] == count) else None


class ProductSession:
    """Own startup, guarded input, capture, cleanup and an honest evidence report.

    Use as a context manager. Call check() only for observed outcomes; capture()
    records pixels as awaiting review. Exceptions publish a failed report and propagate.
    """

    def __init__(self, binary, output, appearance="dark", geometry="normal", scenario="exploratory"):
        self.binary = Path(binary).resolve(strict=True)
        requested_output = Path(output).absolute()
        self.output = requested_output.parent.resolve() / requested_output.name
        if self.output.is_relative_to(Path(__file__).resolve().parent.parent):
            raise ValueError("Artifact output must be outside the checkout")
        self.appearance = appearance
        self.geometry = geometry
        self.process = None
        self.pointer = None
        self.keyboard = None
        self.temporary = None
        self.log = None
        self.stage = "startup"
        planned = ["startup"]
        if scenario in ("search", "activate"):
            planned += ["keyboard_search", "fixture_identity"]
        if scenario == "activate":
            planned.append("safe_activation")
        self.report = dict(format=1, status="incomplete", scenario=scenario,
                           checks=[dict(name=name, status="incomplete") for name in planned], captures=[],
                           requested_appearance=appearance, requested_theme="ikupenov/sonoma-" + appearance + ".json",
                           geometry=geometry, limits=LIMITS, visual_assessment="not_requested",
                           semantic_setup=["Private XDG preferences, catalog and appearance seeded before startup.",
                                           "Owned window focus and geometry set through compositor IPC."])

    def __enter__(self):
        # Do not resolve a symlink at the output leaf or overwrite any prior evidence.
        self.output.mkdir(mode=0o700)
        self._old_handlers = {}
        self._old_umask = os.umask(0o077)
        try:
            for sig in (signal.SIGINT, signal.SIGTERM):
                self._old_handlers[sig] = signal.signal(sig, self._interrupted)
            self._start()
            return self
        except BaseException as error:
            self.__exit__(type(error), error, error.__traceback__)
            raise

    @staticmethod
    def _interrupted(number, frame):
        raise InterruptedError("Verification stopped by signal " + str(number))

    def _start(self):
        if not os.environ.get("WAYLAND_DISPLAY") or os.environ.get("QT_QPA_PLATFORM") == "offscreen":
            raise RuntimeError("Verification needs a native Wayland session")
        self.monitors = json.loads(command("hyprctl", "-j", "monitors"))
        if len(self.monitors) != 1 or self.monitors[0]["transform"] != 0:
            raise RuntimeError("Only one unrotated output is supported; do not change desktop settings")
        monitor = self.monitors[0]
        if monitor.get("disabled") or not monitor.get("dpmsStatus", True):
            raise RuntimeError("The output must be enabled and awake")
        self.report["output"] = {key: monitor[key] for key in ("width", "height", "scale", "transform")}
        version = json.loads(command("hyprctl", "-j", "version"))
        self.report["compositor"] = {key: version.get(key) for key in ("version", "commit")}
        with self.binary.open("rb") as stream:
            self.report["binary_sha256"] = hashlib.file_digest(stream, "sha256").hexdigest()
        self.report["binary"] = str(self.binary)
        self.temporary = tempfile.TemporaryDirectory(prefix="runix-product-")
        self.root = Path(self.temporary.name)
        env = os.environ.copy()
        for name in ("CONFIG", "DATA", "STATE", "CACHE"):
            directory = self.root / name.lower()
            directory.mkdir(mode=0o700)
            env["XDG_" + name + "_HOME"] = str(directory)
        empty = self.root / "empty"
        empty.mkdir(mode=0o700)
        env.update(XDG_DATA_DIRS=str(empty), XDG_CONFIG_DIRS=str(empty),
                   QT_QPA_PLATFORM="wayland", QT_QUICK_BACKEND="software", QT_QUICK_CONTROLS_STYLE="Basic",
                   QT_SCALE_FACTOR="1", QT_SCALE_FACTOR_ROUNDING_POLICY="PassThrough")
        for key in ("QT_SCREEN_SCALE_FACTORS", "QT_AUTO_SCREEN_SCALE_FACTOR"):
            env.pop(key, None)
        config = self.root / "config/runix"
        config.mkdir(mode=0o700)
        write_json(config / "config.json", dict(shortcut_onboarding_dismissed=True, appearance=dict(
            mode=self.appearance, light_id="ikupenov/sonoma-light.json", dark_id="ikupenov/sonoma-dark.json",
            improve_contrast=False)))
        self._fixtures()
        self.endpoint = self.root / "debug.sock"
        self.activation = self.root / "activation.sock"
        self.log = (self.output / "product.log").open("w")
        self.process = subprocess.Popen([str(self.binary), "--disable-desktop-integration",
                                         "--activation-socket", str(self.activation),
                                         "--debug-socket", str(self.endpoint)],
                                        cwd=empty, env=env, stdout=self.log, stderr=self.log, start_new_session=True)
        self.window = ProductWindow(self.endpoint, self.process)
        wait_for(lambda: self.process.poll() is not None or (self.endpoint.exists() and self.window.client()),
                 "the owned native product window")
        if self.process.poll() is not None:
            raise RuntimeError("The product exited during startup; inspect product.log")
        self._geometry()
        command("hyprctl", "dispatch", "hl.dsp.focus({window=" + json.dumps(self.selector) + "})")
        wait_for(self.window.ready, "owned compositor focus")
        def ready():
            self.window.focused()
            snapshot = self.window.snapshot()
            return snapshot if snapshot["ready"] and snapshot["screen"] == "search" else None
        wait_for(ready, "product startup and the search screen")
        self.window.frame()
        self.keyboard = Keyboard().__enter__()
        self.report["native_input"] = "Wayland virtual keyboard with a private US keymap; session layout unchanged."
        self.check("startup", "Private native product is ready on the search screen.")

    @property
    def selector(self):
        return "pid:" + str(self.process.pid)

    def _geometry(self):
        width, height = GEOMETRIES[self.geometry]
        selector = json.dumps(self.selector)
        command("hyprctl", "dispatch", "hl.dsp.window.float({window=" + selector + ",action='enable'})")
        command("hyprctl", "dispatch", "hl.dsp.window.resize({window=" + selector
                + f",x={width},y={height},relative=false" + "})")
        command("hyprctl", "dispatch", "hl.dsp.window.center({window=" + selector + "})")
        previous, since = None, time.monotonic()
        def settled():
            nonlocal previous, since
            client = self.window.client()
            snapshot = self.window.snapshot()
            signature = (tuple(client["at"]), tuple(client["size"]),
                         snapshot["window"]["width"], snapshot["window"]["height"])
            if signature != previous:
                previous, since = signature, time.monotonic()
            return (tuple(client["size"]) == (width, height) and snapshot["rendering"]["frames"] > 0
                    and time.monotonic() - since >= 0.3)
        wait_for(settled, "the selected stable native client geometry")
        client = self.window.client()
        monitor = self.monitors[0]
        x, y = client["at"]
        if not (monitor["x"] <= x and monitor["y"] <= y
                and x + width <= monitor["x"] + monitor["width"] / monitor["scale"]
                and y + height <= monitor["y"] + monitor["height"] / monitor["scale"]):
            raise RuntimeError("The owned client is outside the supported output bounds")

    def _fixtures(self):
        self.launches = self.output / "launches.jsonl"
        helper = self.root / "launch-helper"
        # Fixed bounded helper: no shell, user commands, installed apps or persistent child.
        helper.write_text("#!/usr/bin/python3\nimport json, sys\nwith open(" + repr(str(self.launches))
                          + ", 'a') as stream:\n    stream.write(json.dumps(sys.argv[1:]) + '\\n')\n")
        helper.chmod(0o700)
        directory = self.root / "data/applications"
        directory.mkdir(mode=0o700)
        for suffix, title, comment in (
                ("alpha", "Nativeproof Alpha", ""),
                ("long", "Nativeproof Long Application Name With More Text Than A Narrow Row Can Show", ""),
                ("support", "Nativeproof Support", "Long supporting text " * 12)):
            entry = ("[Desktop Entry]\nType=Application\nName=" + title + "\nComment=" + comment
                     + '\nExec="' + str(helper) + '" ' + suffix + '\nTerminal=false\n')
            (directory / ("nativeproof-" + suffix + ".desktop")).write_text(entry)

    def check(self, name, detail):
        result = dict(name=name, status="passed", detail=detail)
        existing = next((check for check in self.report["checks"] if check["name"] == name), None)
        if existing is not None:
            existing.update(result)
        else:
            self.report["checks"].append(result)

    def key(self, key, ctrl=False):
        self.window.focused()
        self.keyboard.press(key, ctrl=ctrl)

    def search(self, query="nativeproof", count=3):
        self.stage = "native keyboard search"
        if not query.isascii() or not query.isalpha() or not query.islower():
            raise ValueError("Native query input supports lowercase ASCII letters only")
        # Semantic focus is setup; query content is delivered only through native keyboard input.
        self.window.focused()
        self.window.action("search-query", "focus")
        self.report["semantic_setup"].append("Search textbox focused; no semantic query replacement.")
        self.key("a", ctrl=True)
        for letter in query:
            self.key(letter)
        snapshot = wait_for(lambda: self.window.search_ready(query, count),
                            f"native query {query!r} with {count} settled results")
        self.check("keyboard_search", "Native keys produced the requested query and completed result model.")
        return snapshot

    def capture(self, name="search"):
        self.stage = "native capture"
        if not name or any(char not in "abcdefghijklmnopqrstuvwxyz0123456789-_" for char in name):
            raise ValueError("Capture names use lowercase letters, digits, hyphens and underscores")
        image = self.output / (name + ".png")
        if image.exists():
            raise FileExistsError("Capture already exists: " + str(image))
        self.window.focused()
        snapshot = self.window.frame()
        self.window.focused()
        client = self.window.client()
        x, y = client["at"]
        width, height = client["size"]
        command("grim", "-g", f"{x},{y} {width}x{height}", str(image))
        self.window.focused()
        write_json(self.output / (name + ".json"), snapshot)
        self.report["captures"].append(dict(image=image.name, snapshot=name + ".json",
            status="awaiting_visual_assessment", window=snapshot["window"], rendering=snapshot["rendering"],
            compositor_bounds=dict(at=client["at"], size=client["size"])))
        return snapshot

    def activate(self, mode):
        self.stage = "native " + mode + " activation"
        snapshot = self.window.snapshot()
        state = snapshot["search"]["state"]
        if (state["selected_id"] != FIXTURE_ID or state["loading"] or not state["activation_enabled"]
                or state["generation"] != state["applied_generation"]):
            raise RuntimeError("The exact safe fixture is not selected and ready")
        if self.launches.exists():
            raise RuntimeError("A fixture ran before the activation check")
        if mode == "keyboard":
            self.key("Return")
        elif mode == "pointer":
            self.pointer = Pointer().__enter__()
            self.window.point(self.pointer, self.monitors, "search-primary-action")
            self.window.focused()
            self.pointer.button(True)
            self.window.focused()
            self.pointer.button(False)
        else:
            raise ValueError("Activation mode must be keyboard or pointer")
        def launched():
            snapshot = self.window.snapshot()
            return snapshot if self.launches.exists() and not snapshot["window"]["visible"] else None
        snapshot = wait_for(launched, "the harmless helper and hidden product window")
        if self.launches.read_text().splitlines() != ['["alpha"]']:
            raise RuntimeError("Expected exactly one launch of the alpha fixture")
        wait_for(lambda: not self.window.client(), "the unmapped product client after launch")
        write_json(self.output / "after-launch.json", snapshot)
        self.check("safe_activation", "Native " + mode + " input ran only the alpha helper once and hid the window.")

    def __exit__(self, kind, error, traceback):
        cleanup_errors = []
        if error:
            self.report["failure"] = dict(stage=self.stage, message=str(error))
            if self.process and self.process.poll() is None and self.endpoint.exists():
                try:
                    write_json(self.output / "failure-snapshot.json", self.window.snapshot())
                except Exception as diagnostic_error:
                    self.report["failure"]["snapshot_error"] = str(diagnostic_error)
        try:
            if self.pointer:
                self.pointer.close()
        except Exception as cleanup_error:
            cleanup_errors.append(str(cleanup_error))
        try:
            if self.keyboard:
                self.keyboard.close()
        except Exception as cleanup_error:
            cleanup_errors.append(str(cleanup_error))
        try:
            if self.process:
                if self.process.poll() is None:
                    self.process.terminate()
                    try:
                        self.process.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        self.process.kill()
                        self.process.wait(timeout=5)
                # This session created its own process group; never target unrelated children.
                try:
                    os.killpg(self.process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                if any(check["name"] == "safe_activation" and check["status"] == "passed"
                       for check in self.report["checks"]):
                    if self.launches.read_text().splitlines() != ['["alpha"]']:
                        raise RuntimeError("The final helper record is not exactly one alpha launch")
        except Exception as cleanup_error:
            cleanup_errors.append(str(cleanup_error))
        try:
            if self.log:
                self.log.close()
            if self.temporary:
                root = Path(self.temporary.name)
                self.temporary.cleanup()
                if root.exists():
                    raise RuntimeError("Disposable product state remains")
        except Exception as cleanup_error:
            cleanup_errors.append(str(cleanup_error))
        for sig, handler in self._old_handlers.items():
            signal.signal(sig, handler)
        self.report["cleanup"] = dict(status="failed" if cleanup_errors else "passed", errors=cleanup_errors,
                                       child_reaped=self.process is None or self.process.poll() is not None,
                                       disposable_state_removed=self.temporary is None or not Path(self.temporary.name).exists())
        incomplete = any(check["status"] != "passed" for check in self.report["checks"])
        self.report["status"] = "failed" if error or cleanup_errors or incomplete else "passed"
        self.report["status_scope"] = "Programmatic checks only; visual approval is separate."
        try:
            write_json(self.output / "report.json", self.report)
            lines = ["# Product verification", "", "Programmatic status: " + self.report["status"], "",
                     "Scenario: " + self.report["scenario"], ""]
            lines += ["- " + check["status"].upper() + ": " + check.get("detail", check["name"])
                      for check in self.report["checks"]]
            if error:
                lines += ["- FAILED/INCOMPLETE at " + self.stage + ": " + str(error)]
            lines += ["- Cleanup: " + self.report["cleanup"]["status"]] + ["- " + item for item in cleanup_errors]
            lines += ["", "## Screenshots awaiting visual assessment", ""]
            lines += ["- " + item["image"] for item in self.report["captures"]]
            lines += ["", "## Coverage limits", ""] + ["- " + item for item in LIMITS]
            (self.output / "report.md").write_text("\n".join(lines) + "\n")
        finally:
            os.umask(self._old_umask)
        if (cleanup_errors or incomplete) and not error:
            raise RuntimeError("Verification is incomplete or cleanup failed; inspect report.json")
        return False


def run_scenario(session, scenario, activation):
    session.search()
    snapshot = session.capture()
    state = snapshot["search"]["state"]
    rows = snapshot["search"]["visible_rows"]
    if state["selected_id"] != FIXTURE_ID or not any(row["id"] == FIXTURE_ID and row["selected"] for row in rows):
        raise RuntimeError("The expected alpha fixture is not visibly selected")
    if state["count"] != 3 or len(rows) != 3:
        raise RuntimeError("The bounded long-text and empty-support fixtures are not all visible")
    session.check("fixture_identity", "All three fixture rows are visible; the exact alpha identity is selected.")
    if scenario == "activate":
        session.activate(activation)
