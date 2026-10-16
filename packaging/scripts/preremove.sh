#!/bin/sh
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
set -e

if [ "${1:-}" = "remove" ] || [ "${1:-0}" = "0" ]; then
    if command -v systemctl >/dev/null 2>&1; then
        systemctl stop mcharbor.service || true
        systemctl disable mcharbor.service || true
    fi
fi
