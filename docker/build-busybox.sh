#!/bin/sh
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
set -eu

arch="$1"
src="${2:-/src/busybox}"
out="${3:-/out}"

cd "$src"
make -s defconfig
sed -i \
    -e 's/^# CONFIG_STATIC is not set$/CONFIG_STATIC=y/' \
    -e 's/^# CONFIG_FEATURE_PREFER_APPLETS is not set$/CONFIG_FEATURE_PREFER_APPLETS=y/' \
    -e 's/^# CONFIG_FEATURE_SH_STANDALONE is not set$/CONFIG_FEATURE_SH_STANDALONE=y/' \
    -e 's/^CONFIG_TC=y$/# CONFIG_TC is not set/' \
    -e 's/^CONFIG_FEATURE_TC_INGRESS=y$/# CONFIG_FEATURE_TC_INGRESS is not set/' \
    .config
yes "" | make -s oldconfig >/dev/null
for opt in CONFIG_STATIC CONFIG_FEATURE_PREFER_APPLETS CONFIG_FEATURE_SH_STANDALONE; do
    grep -qx "$opt=y" .config
done
if grep -qx 'CONFIG_TC=y' .config; then
    echo "busybox-$arch still selects tc, which does not build against current kernel headers" >&2
    exit 1
fi
make -s -j"$(nproc)" busybox
if readelf -l busybox | grep -q INTERP; then
    echo "busybox-$arch is dynamically linked" >&2
    exit 1
fi
./busybox sh -c 'ls / >/dev/null && echo ok' | grep -qx ok
mkdir -p "$out"
install -m 0755 busybox "$out/busybox-$arch"
cp .config "$out/busybox-$arch.config"
sha256sum "$out/busybox-$arch"
