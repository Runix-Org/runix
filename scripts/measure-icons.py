#!/usr/bin/env python3
"""Exercise the live icon cache; rendered pixels still require visual inspection."""
import argparse
import json
import socket
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--socket", required=True)
args = parser.parse_args()


def request(op, **fields):
    data = dict(version=1, id="icons", op=op, deadline_ms=int(time.time() * 1000) + 1500, **fields)
    with socket.socket(socket.AF_UNIX) as client:
        client.settimeout(2)
        client.connect(args.socket)
        client.sendall((json.dumps(data) + "\n").encode())
        result = json.loads(client.makefile().readline())
    if "error" in result:
        raise RuntimeError(result["error"])
    return result["result"]


def action(target, name, text=""):
    return request("action", target=target, action=name, text=text)


def model():
    snapshot = request("snapshot")
    return next(control["model"] for control in snapshot["controls"] if control["id"] == "results")


def reload_results():
    previous = model()["state"]["generation"]
    action("run-query", "activate")
    deadline = time.monotonic() + 2
    while True:
        state = model()["state"]
        if (not state["loading"] and state["generation"] > previous
                and state["generation"] == state["applied_generation"]):
            break
        if time.monotonic() > deadline:
            raise RuntimeError("The latest query did not reach the model within two seconds")
        time.sleep(0.001)
    time.sleep(0.025)  # Let QML lay out the reset view before sampling image counters.


initial = model()["state"]
if initial["loading"] or initial["query"] or initial["count"] != 200:
    raise RuntimeError("Start with Search mode, an empty query, and 200 loaded results")

maximum_entries = maximum_bytes = maximum_delegates = 0
for cycle in range(3):
    for index in range(0, 200, 4):
        action("results", "scroll", str(index))
        time.sleep(0.025)
        current = model()
        stats = current["state"]["icons"]
        maximum_entries = max(maximum_entries, stats["entries"])
        maximum_bytes = max(maximum_bytes, stats["bytes"])
        maximum_delegates = max(maximum_delegates, current["delegates"])
        assert stats["entries"] <= 64, stats
        assert current["visible_rows"], current
        assert all(row["icon_status"] == 1 for row in current["visible_rows"]), current
    reload_results()

action("results", "scroll", "0")
time.sleep(0.15)
before = model()["state"]["icons"]
for _ in range(8):
    reload_results()
after = model()["state"]["icons"]
assert after["lookups"] == before["lookups"], (before, after)
print(json.dumps(dict(maximum_entries=maximum_entries, maximum_url_bytes=maximum_bytes,
                      maximum_delegates=maximum_delegates, before_reload=before, after_reload=after), indent=2))
