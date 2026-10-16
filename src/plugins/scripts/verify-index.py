#!/usr/bin/env python3
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
"""Verify the signature on index.yaml."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

try:
    from nacl.signing import VerifyKey
    from nacl.encoding import RawEncoder
    import yaml
except ImportError:
    sys.stderr.write("pynacl and pyyaml are required: pip install pynacl pyyaml\n")
    sys.exit(2)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("index", type=Path, help="path to index.yaml")
    ap.add_argument("pubkey", type=Path, help="path to ed25519 public key (32 raw bytes)")
    args = ap.parse_args()

    raw = args.index.read_bytes()
    doc = yaml.safe_load(raw)
    sig_block = doc.get("plugin_index_signature")
    if not sig_block:
        sys.stderr.write("no plugin_index_signature block\n")
        return 1

    pub = args.pubkey.read_bytes()
    if len(pub) != 32:
        sys.stderr.write(f"public key must be 32 raw bytes, got {len(pub)}\n")
        return 2

    # Recompute body without signature.
    doc.pop("plugin_index_signature")
    body = json.dumps(doc, sort_keys=True, separators=(",", ":")).encode("utf-8")

    try:
        VerifyKey(pub, encoder=RawEncoder).verify(body, bytes.fromhex(sig_block["value"]))
    except Exception as exc:
        sys.stderr.write(f"signature INVALID: {exc}\n")
        return 1

    print(f"signature OK  (key id={sig_block.get('public_key_id')})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())