// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useComputeNetworks, useComputePools } from './useComputeResources';
import { useMigrationTargets, type MigrateInput, type MigrateNIC } from './useMigration';
import { nicAttachment } from '../utils/nicAttachment';
import type { VM } from '../types';

/** Positive whole numbers only; anything else is sent as "no limit". */
function limit(value: number | null): number | undefined {
  return value !== null && Number.isInteger(value) && value > 0 ? value : undefined;
}

/**
 * The migrate dialog's draft: where the guest goes and how.
 *
 * The pool and the networks are the target's, so both reset when the target
 * changes — a pool id from the old host would name nothing on the new one.
 */
export function useMigrateForm(vm: VM, enabled: boolean, liveAllowed: boolean) {
  const targets = useMigrationTargets(vm.environmentId, enabled);
  const [pickedTarget, setTarget] = useState('');
  const [pickedPoolId, setPoolId] = useState('');
  const [live, setLive] = useState(false);
  const [targetAddress, setTargetAddress] = useState('');
  const [maxBandwidthMb, setMaxBandwidthMb] = useState<number | null>(null);
  const [maxDowntimeMs, setMaxDowntimeMs] = useState<number | null>(null);
  const [nicNetworks, setNicNetworks] = useState<Record<string, string>>({});

  // With one possible target there is nothing to choose.
  const onlyTarget = targets.data?.length === 1 ? targets.data[0]?.environmentId : undefined;
  const target = pickedTarget || onlyTarget || '';

  const pools = useComputePools(target || undefined);
  const networks = useComputeNetworks(target || undefined);

  const chooseTarget = (next: string) => {
    setTarget(next);
    setPoolId('');
    setNicNetworks({});
  };

  // The target's default pool is the one an operator would pick nine times in ten.
  const preferredPool = pools.data?.find((entry) => entry.isDefault) ?? pools.data?.[0];
  const poolId = pickedPoolId || preferredPool?.id || '';

  if (!liveAllowed && live) setLive(false);

  const pool = pools.data?.find((entry) => entry.id === poolId);
  const ready = Boolean(target && pool);

  const body = useMemo((): MigrateInput | null => {
    if (!target || !pool) return null;
    const nics: MigrateNIC[] = [];
    for (const nic of vm.nics ?? []) {
      const network = networks.data?.find((entry) => entry.id === nicNetworks[nic.id]);
      if (network) nics.push({ nicId: nic.id, networkId: network.id, ...nicAttachment(network) });
    }
    return {
      targetEnvironmentId: target,
      targetPoolId: pool.id,
      targetPoolBackend: pool.backend,
      targetPoolTarget: pool.target,
      live: live && liveAllowed,
      targetAddress: live && targetAddress.trim() ? targetAddress.trim() : undefined,
      maxBandwidthMb: live ? limit(maxBandwidthMb) : undefined,
      maxDowntimeMs: live ? limit(maxDowntimeMs) : undefined,
      nics: nics.length ? nics : undefined,
    };
  }, [
    target,
    pool,
    vm.nics,
    networks.data,
    nicNetworks,
    live,
    liveAllowed,
    targetAddress,
    maxBandwidthMb,
    maxDowntimeMs,
  ]);

  return {
    targets,
    target,
    chooseTarget,
    pools,
    poolId,
    setPoolId,
    pool,
    networks,
    nicNetworks,
    setNicNetwork: (nicId: string, networkId: string) =>
      setNicNetworks((current) => ({ ...current, [nicId]: networkId })),
    live,
    setLive,
    targetAddress,
    setTargetAddress,
    maxBandwidthMb,
    setMaxBandwidthMb,
    maxDowntimeMs,
    setMaxDowntimeMs,
    ready,
    body,
  };
}

export type MigrateForm = ReturnType<typeof useMigrateForm>;
