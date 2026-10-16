// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import type { ComputePool } from './useComputePools';
import type { RecreateBody, RecreateDraft } from './useRecreate';
import type { KeyedMount, KeyedNIC } from '../components/RecreateDefinitionFields';
import type { InitSystem } from '../types';

const MIB = 1024 * 1024;

function keyedNICs(draft: RecreateDraft): KeyedNIC[] {
  return (draft.create.nics ?? []).map((nic, position) => ({
    ...nic,
    key: `nic-${position}-${nic.guestName ?? ''}`,
  }));
}

function keyedMounts(draft: RecreateDraft): KeyedMount[] {
  return (draft.create.mounts ?? []).map((mount, position) => ({
    ...mount,
    key: `mount-${position}-${mount.destination}`,
  }));
}

function strip<T extends { key: string }>(entries: T[]): Omit<T, 'key'>[] {
  return entries.map(({ key: _key, ...rest }) => rest);
}

export function matchPool(draft: RecreateDraft, pools: ComputePool[]): ComputePool | undefined {
  return pools.find(
    (pool) =>
      pool.target === draft.create.poolTarget &&
      (!draft.create.poolBackend || pool.backend === draft.create.poolBackend),
  );
}

export function useRecreateForm(draft: RecreateDraft) {
  const limits = draft.create.limits ?? { swapMaxBytes: 0 };
  const [environmentId, setEnvironmentId] = useState(draft.create.environmentId);
  const [containerName, setContainerName] = useState(draft.create.name);
  const [poolChoice, setPoolChoice] = useState<string | null>(null);
  const [initSystem, setInitSystem] = useState<InitSystem>(draft.create.initSystem ?? 'systemd');
  const [privileged, setPrivileged] = useState(Boolean(draft.create.privileged));
  const [nesting, setNesting] = useState(Boolean(draft.create.nesting));
  const [memoryMb, setMemoryMb] = useState(Math.round((limits.memoryMaxBytes ?? 0) / MIB));
  const [cpuMax, setCpuMax] = useState(limits.cpuMax ?? '');
  const [pidsMax, setPidsMax] = useState(limits.pidsMax ?? 0);
  const [nics, setNics] = useState<KeyedNIC[]>(() => keyedNICs(draft));
  const [mounts, setMounts] = useState<KeyedMount[]>(() => keyedMounts(draft));
  const [startAfter, setStartAfter] = useState(false);

  const resolvePool = (pools: ComputePool[]): ComputePool | undefined =>
    poolChoice === null ? matchPool(draft, pools) : pools.find((pool) => pool.id === poolChoice);

  const buildBody = (pool: ComputePool): RecreateBody => ({
    create: {
      ...draft.create,
      environmentId,
      name: containerName.trim(),
      poolId: pool.id,
      poolBackend: pool.backend,
      poolTarget: pool.target,
      initSystem,
      privileged,
      nesting,
      limits: {
        ...limits,
        memoryMaxBytes: memoryMb > 0 ? memoryMb * MIB : undefined,
        cpuMax: cpuMax.trim() || undefined,
        pidsMax: pidsMax > 0 ? pidsMax : undefined,
      },
      nics: strip(nics),
      mounts: strip(mounts),
    },
    osFamily: draft.osFamily,
    osVersion: draft.osVersion,
    startAfterRestore: startAfter,
  });

  return {
    environmentId,
    setEnvironmentId: (value: string) => {
      setEnvironmentId(value);
      setPoolChoice('');
    },
    name: containerName,
    setName: setContainerName,
    setPoolChoice,
    resolvePool,
    initSystem,
    setInitSystem,
    privileged,
    setPrivileged,
    nesting,
    setNesting,
    memoryMb,
    setMemoryMb,
    cpuMax,
    setCpuMax,
    pidsMax,
    setPidsMax,
    nics,
    setNics,
    mounts,
    setMounts,
    startAfter,
    setStartAfter,
    buildBody,
  };
}
