#!/bin/sh
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
set -e

if [ "${1:-}" = "remove" ] || [ "${1:-0}" = "0" ]; then
    if command -v systemctl >/dev/null 2>&1; then
        systemctl daemon-reload || true
    fi
fi

if [ "${1:-}" = "purge" ]; then
    rm -f /etc/mcharbor/install-kind.env
    rm -f /etc/systemd/system/mcharbor.service.d/run-as-root.conf /etc/systemd/system/mcharbor.service.d/run-as-mcharbor.conf
    rm -rf /var/lib/mcharbor
    rm -rf /etc/mcharbor
    if getent passwd mcharbor >/dev/null 2>&1; then
        userdel mcharbor 2>/dev/null || deluser mcharbor 2>/dev/null || true
    fi
fi
