#!/usr/bin/env python3
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
"""Sign index.yaml in place.

The signature is written into the `plugin_index_signature.value` field.
Algorithm: ed25519 (PyNaCl). The public-key id (`mcsparrow-2026-01`) must
match the entry in McHarbor's `core/plugin/keys.go`.

Usage:
    python3 scripts/sign-index.py index.yaml signing/key.pem
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path

try:
    from nacl.signing import SigningKey
    from nacl.encoding import RawEncoder
except ImportError:
    sys.stderr.write("pynacl is required: pip install pynacl\n")
    sys.exit(2)


def canonicalize(data: bytes) -> bytes:
    """Canonical-JSON: sorted keys, no whitespace, UTF-8.

    PyYAML's `yaml.safe_load` is fine for our schema; we re-dump via
    json.dumps(sort_keys=True, separators=(',', ':')) to get a stable
    byte sequence regardless of whitespace in the source file.
    """
    import yaml  # local import so PyYAML is only needed at sign time

    obj = yaml.safe_load(data)
    return json.dumps(obj, sort_keys=True, separators=(",", ":")).encode("utf-8")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("index", type=Path, help="path to index.yaml")
    ap.add_argument("key", type=Path, help="path to ed25519 private key (32 raw bytes)")
    ap.add_argument(
        "--public-key-id",
        default="mcsparrow-2026-01",
        help="public-key id to embed (default: mcsparrow-2026-01)",
    )
    args = ap.parse_args()

    raw = args.index.read_bytes()
    body = canonicalize(raw)

    # Strip any prior signature so we re-sign the rest only.
    import yaml

    doc = yaml.safe_load(raw)
    doc.pop("plugin_index_signature", None)
    body = json.dumps(doc, sort_keys=True, separators=(",", ":")).encode("utf-8")

    seed = args.key.read_bytes()
    if len(seed) != 32:
        sys.stderr.write(f"key must be 32 raw bytes, got {len(seed)}\n")
        return 2

    sk = SigningKey(seed, encoder=RawEncoder)
    sig = sk.sign(body).signature

    digest = hashlib.sha256(raw).hexdigest()

    # Inject signature block. We do this by string replacement so we
    # preserve the YAML formatting around it.
    text = args.index.read_text(encoding="utf-8")
    sig_block = (
        "plugin_index_signature:\n"
        f"  algorithm: \"ed25519\"\n"
        f"  public_key_id: \"{args.public_key_id}\"\n"
        f"  value: \"{sig.hex()}\"\n"
        f"  body_sha256: \"{digest}\"\n"
    )

    sentinel = "plugin_index_signature:"
    if sentinel in text:
        head, _, tail = text.partition(sentinel)
        # tail contains everything from sentinel to end; drop those lines
        tail_lines = tail.splitlines()
        # find first blank-line or new top-level key after sentinel
        keep_start = len(tail_lines)
        for i, line in enumerate(tail_lines[1:], start=1):
            if line and not line.startswith((" ", "\t", "#")):
                keep_start = i
                break
        text = head + sig_block + "\n".join(tail_lines[keep_start:]) + "\n"
    else:
        text = text.rstrip() + "\n\n" + sig_block

    args.index.write_text(text, encoding="utf-8")
    print(f"signed {args.index}  (sha256={digest[:16]}..., sig={sig.hex()[:16]}...)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())