# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.

import argparse
import json
import os
import sys
import tempfile

MAX_LEN = 256
ZERO = {"core": ("0", "0", "0"), "pre": ()}


def _ascii_digits(s):
    return s != "" and all("0" <= c <= "9" for c in s)


def _idents_ok(s, allow_hyphen):
    if s == "":
        return False
    for ident in s.split("."):
        if ident == "":
            return False
        for c in ident:
            ok = "0" <= c <= "9" or "a" <= c <= "z" or "A" <= c <= "Z" or (allow_hyphen and c == "-")
            if not ok:
                return False
    return True


def parse(s):
    if not isinstance(s, str) or s == "" or len(s) > MAX_LEN:
        return None
    rest, pre = s, ()
    if "+" in rest:
        rest, meta = rest.split("+", 1)
        if not _idents_ok(meta, True):
            return None
    if "-" in rest:
        rest, raw = rest.split("-", 1)
        if not _idents_ok(raw, False):
            return None
        pre = tuple(raw.split("."))
    parts = rest.split(".")
    if len(parts) != 3 or not all(_ascii_digits(p) for p in parts):
        return None
    return {"core": tuple(p.lstrip("0") or "0" for p in parts), "pre": pre}


def is_prerelease(s):
    v = parse(s)
    return v is not None and len(v["pre"]) > 0


def _num(a, b):
    if len(a) != len(b):
        return -1 if len(a) < len(b) else 1
    return (a > b) - (a < b)


def _ident(a, b):
    an, bn = _ascii_digits(a), _ascii_digits(b)
    if an and bn:
        return _num(a.lstrip("0") or "0", b.lstrip("0") or "0")
    if an:
        return -1
    if bn:
        return 1
    return (a > b) - (a < b)


def compare(a, b):
    va, vb = parse(a) or ZERO, parse(b) or ZERO
    for x, y in zip(va["core"], vb["core"]):
        c = _num(x, y)
        if c:
            return c
    pa, pb = va["pre"], vb["pre"]
    if not pa and not pb:
        return 0
    if not pa:
        return 1
    if not pb:
        return -1
    for x, y in zip(pa, pb):
        c = _ident(x, y)
        if c:
            return c
    return (len(pa) > len(pb)) - (len(pa) < len(pb))


def _usable(row):
    return (
        isinstance(row, dict)
        and isinstance(row.get("version"), str)
        and parse(row["version"]) is not None
        and isinstance(row.get("download_url"), str)
        and row["download_url"] != ""
    )


def _newest(rows):
    best = None
    for row in rows:
        if best is None or compare(row["version"], best["version"]) >= 0:
            best = row
    return best


def merge_versions(existing, incoming):
    """Return (stable, prerelease) rows: the newest stable, and the newest
    prerelease only while it is newer than that stable."""
    rows = [r for r in existing if _usable(r) and compare(r["version"], incoming["version"]) != 0]
    rows.append(incoming)
    stable = _newest([r for r in rows if not is_prerelease(r["version"])])
    pre = _newest([r for r in rows if is_prerelease(r["version"])])
    if stable is not None and pre is not None and compare(pre["version"], stable["version"]) <= 0:
        pre = None
    kept = [r for r in (stable, pre) if r is not None]
    if not any(r is incoming for r in kept):
        raise ValueError(
            "version %s is older than what the catalog already holds (%s)"
            % (incoming["version"], ", ".join(r["version"] for r in kept))
        )
    return ([stable] if stable is not None else []), ([pre] if pre is not None else [])


def _scrape_manifest(path):
    entry = {}
    wanted = ("name", "author", "license", "homepage", "description", "category")
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            key, sep, raw = line.partition(":")
            if not sep or key != key.strip() or key.strip() not in wanted:
                continue
            value = raw.strip().strip('"')
            if value and value not in ("|", ">"):
                entry.setdefault(key.strip(), value)
    return entry


def _load_json(path):
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


def write_entry(args):
    if parse(args.version) is None:
        raise ValueError("version %r is not MAJOR.MINOR.PATCH[-suffix]" % args.version)
    if parse(args.min_mcharbor) is None:
        raise ValueError("minimum host %r is not MAJOR.MINOR.PATCH[-suffix]" % args.min_mcharbor)
    existing = []
    if os.path.isfile(args.out):
        try:
            prior = _load_json(args.out)
            existing = list(prior.get("versions", [])) + list(prior.get("prerelease_versions", []))
        except (ValueError, AttributeError, TypeError) as exc:
            raise ValueError("existing %s is unreadable: %s" % (args.out, exc))

    if os.path.isfile(args.src):
        entry = _load_json(args.src)
    else:
        entry = _scrape_manifest(args.manifest)

    entry["slug"] = args.slug
    entry.setdefault("name", args.slug)
    entry["icon"] = "plugin.svg"
    entry.pop("versions", None)
    entry.pop("prerelease_versions", None)
    row = {
        "version": args.version,
        "build": args.build,
        "download_url": args.url,
        "min_mcharbor": args.min_mcharbor,
        "signature": args.signature,
    }
    stable, pre = merge_versions(existing, row)
    entry["versions"] = stable
    if pre:
        entry["prerelease_versions"] = pre

    out_dir = os.path.dirname(os.path.abspath(args.out))
    fd, tmp = tempfile.mkstemp(dir=out_dir, suffix=".tmp")
    try:
        with os.fdopen(fd, "w", encoding="utf-8", newline="\n") as fh:
            json.dump(entry, fh, indent=2, ensure_ascii=False)
            fh.write("\n")
        os.replace(tmp, args.out)
    except BaseException:
        if os.path.exists(tmp):
            os.unlink(tmp)
        raise


def main(argv):
    top = argparse.ArgumentParser(prog="catalog_entry.py")
    top.add_argument("--compare", nargs=2, metavar=("A", "B"))
    top.add_argument("--parse", metavar="V")
    top.add_argument("--prerelease", metavar="V")
    sub = top.add_subparsers(dest="cmd")
    w = sub.add_parser("write")
    for name in ("src", "out", "slug", "version", "url", "manifest", "min-mcharbor",
                 "signature", "build"):
        w.add_argument("--" + name, required=True)
    args = top.parse_args(argv)
    if args.compare:
        print(compare(*args.compare))
    elif args.parse is not None:
        print("true" if parse(args.parse) is not None else "false")
    elif args.prerelease is not None:
        print("true" if is_prerelease(args.prerelease) else "false")
    elif args.cmd == "write":
        try:
            write_entry(args)
        except (ValueError, OSError) as exc:
            print("catalog_entry: %s" % exc, file=sys.stderr)
            return 2
    else:
        top.print_usage(sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
