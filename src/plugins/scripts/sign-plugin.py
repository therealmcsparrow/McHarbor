#!/usr/bin/env python3
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
"""Sign a plugin tarball with ed25519.

Usage:
    python scripts/sign-plugin.py \\
        --tarball dist/mcharbor-plugin-hello-world-1.0.0.tar.gz \\
        --key signing/plugin-signing.key

Writes the signature as a sibling file:

    dist/mcharbor-plugin-hello-world-1.0.0.tar.gz.sig

Format: 64 bytes of raw ed25519 signature, base64-encoded (single-line, no
trailing newline). This matches what `modules/appstore/plugins/installer.go`
expects when verifying a downloaded package.

The signing key must live under `signing/` and is gitignored. Keep the
public half (`.pub`) inside the repo so contributors can verify locally;
never commit the private half (`.key`).
"""
from __future__ import annotations

import argparse
import base64
import sys
from pathlib import Path

from nacl import signing  # type: ignore[import-not-found]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Sign a McHarbor plugin tarball.")
    parser.add_argument(
        "--tarball",
        required=True,
        type=Path,
        help="Path to the plugin .tar.gz produced by `make build`.",
    )
    parser.add_argument(
        "--key",
        required=True,
        type=Path,
        help="Path to the ed25519 private key (must not be committed).",
    )
    return parser.parse_args()


def load_key(path: Path) -> signing.SigningKey:
    if not path.is_file():
        sys.exit(f"signing key not found: {path}")
    raw = path.read_bytes().strip()
    try:
        return signing.SigningKey(raw)
    except Exception as exc:  # pragma: no cover - defensive
        sys.exit(f"failed to load signing key {path}: {exc}")


def sign(tarball: Path, key: signing.SigningKey) -> bytes:
    payload = tarball.read_bytes()
    signed = key.sign(payload)
    return base64.b64encode(signed.signature)


def main() -> int:
    args = parse_args()
    if not args.tarball.is_file():
        sys.exit(f"tarball not found: {args.tarball}")

    key = load_key(args.key)
    signature = sign(args.tarball, key)

    out = args.tarball.with_suffix(args.tarball.suffix + ".sig")
    out.write_bytes(signature + b"\n")
    print(f"wrote {out} ({len(signature)} bytes base64)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
