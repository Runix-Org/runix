#!/usr/bin/env python3
"""Run a bounded isolated product scenario on native Hyprland Wayland."""

import argparse
import os
from pathlib import Path
import sys

sys.dont_write_bytecode = True

from product_verification import ProductSession, run_scenario


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--product", required=True, help="Absolute path to an explicitly built devtools product")
    parser.add_argument("--output", required=True, help="New private artifact directory outside the checkout")
    parser.add_argument("--scenario", choices=("search", "activate"), default="search",
                        help="Capture search results, or also run the exact safe fixture")
    parser.add_argument("--activation", choices=("keyboard", "pointer"), default="keyboard",
                        help="Native activation input for the activate scenario")
    parser.add_argument("--appearance", choices=("light", "dark"), default="dark")
    parser.add_argument("--geometry", choices=("normal", "constrained"), default="normal",
                        help="Owned compositor client size: 640x420 or 520x360")
    args = parser.parse_args()
    if not Path(args.product).is_absolute() or not Path(args.output).is_absolute():
        parser.error("--product and --output must be absolute paths")
    try:
        with ProductSession(args.product, args.output, args.appearance, args.geometry, args.scenario) as session:
            session.report["activation"] = args.activation if args.scenario == "activate" else None
            run_scenario(session, args.scenario, args.activation)
    except (Exception, KeyboardInterrupt) as error:
        parser.exit(1, f"Verification failed: {error}\n")
    print(Path(args.output) / "report.json")


if __name__ == "__main__":
    os.umask(0o077)
    main()
