#!/usr/bin/env python3
"""Measure 100 warmed query-to-model updates through the opt-in debug socket."""

import argparse
import json
import math
import socket
import statistics
import time


def request(path, op, **fields):
    message = dict(version=1, id="search-measure", op=op,
                   deadline_ms=int(time.time() * 1000) + 1500, **fields)
    with socket.socket(socket.AF_UNIX) as connection:
        connection.settimeout(2)
        connection.connect(path)
        connection.sendall(json.dumps(message).encode() + b"\n")
        with connection.makefile("rb") as stream:
            line = stream.readline(65537)
    if not line.endswith(b"\n") or len(line) > 65536:
        raise RuntimeError("The debug response exceeds the frame limit")
    response = json.loads(line)
    if "error" in response:
        raise RuntimeError(response["error"]["message"])
    return response["result"]


def results(path):
    controls = request(path, "snapshot")["controls"]
    return next(control["model"] for control in controls if control["id"] == "results")


def measure(path):
    queries = ["atlas", "bridge", "workspace 03", "document", "record-00499",
               "no-such-match", "", "console project 010", "help", "bookmark"]
    samples = []
    delegates = 0
    work = dict(Active=0, Pending=0, Delivery=0)
    for index in range(120):
        query = queries[index % len(queries)]
        request(path, "action", target="query", action="replace", text=query)
        deadline = time.monotonic() + 2
        while True:
            model = results(path)
            state = model["state"]
            if (not state["loading"] and state["query"] == query
                    and state["generation"] == state["applied_generation"]):
                break
            if time.monotonic() > deadline:
                raise RuntimeError("The latest query did not reach the model within two seconds")
            time.sleep(0.001)
        if index >= 20:
            samples.append(state["latency_ms"])
            delegates = max(delegates, model["delegates"])
            for name, value in state["work"].items():
                work[name] = max(work[name], value)
    ordered = sorted(samples)
    print(json.dumps(dict(updates=len(samples), warmup=20,
                          p50_ms=statistics.median(samples),
                          p95_ms=ordered[math.ceil(len(ordered) * 0.95) - 1],
                          max_ms=max(samples), max_delegates=delegates,
                          observed_settled_work=work), indent=2))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--socket", required=True, help="Connect to the enabled private debug socket")
    arguments = parser.parse_args()
    try:
        measure(arguments.socket)
    except (OSError, RuntimeError, ValueError, KeyError, StopIteration) as error:
        parser.exit(1, f"Search measurement failed: {error}\n")
