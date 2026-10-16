// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useUpdateVM, useVMs } from '../../hooks/useVMs';
import { useComputeCapability } from '../../hooks/useComputeCapability';
import { guestNameErrorKey } from '../../guestName';
import { hostLimits, vcpuZones } from '../../hostLimits';
import type { VM } from '../../types';
import { GROUP_FIELDS, draftFrom, type Draft, type GroupName } from './settingsDraft';

export function useVmSettingsForm(vm: VM) {
  const update = useUpdateVM();
  const { data: capability } = useComputeCapability(vm.environmentId);
  const original = useMemo(() => draftFrom(vm), [vm]);
  const [draft, setDraft] = useState<Draft>(original);
  const [baseline, setBaseline] = useState<Draft>(original);

  if (baseline !== original && update.isIdle) {
    setBaseline(original);
    setDraft(original);
  }

  const editable = vm.editableNow ?? [];
  const set = <K extends keyof Draft>(field: K, value: Draft[K]) =>
    setDraft((current) => ({ ...current, [field]: value }));

  // What the other guests on this host are already holding. Only the running
  // ones: a stopped guest's memory is genuinely free until it starts, and a
  // ceiling that pretended otherwise would be unusable on a host with a few
  // parked machines.
  const { data: hostVMs } = useVMs(vm.environmentId);
  const otherAllocatedMb = useMemo(
    () =>
      (hostVMs ?? [])
        .filter((guest) => guest.id !== vm.id && guest.state === 'running')
        .reduce((total, guest) => total + (guest.memoryMb ?? 0), 0),
    [hostVMs, vm.id],
  );

  const otherAllocatedVCPUs = useMemo(
    () =>
      (hostVMs ?? [])
        .filter((guest) => guest.id !== vm.id && guest.state === 'running')
        .reduce((total, guest) => total + (guest.vcpus ?? 0), 0),
    [hostVMs, vm.id],
  );

  const limits = hostLimits(capability, draft, otherAllocatedMb, otherAllocatedVCPUs);
  const zones = vcpuZones(limits);
  const nameErrorKey = draft.name.trim() === '' ? null : guestNameErrorKey(draft.name.trim());

  const locked = (group: GroupName) => !GROUP_FIELDS[group].every((field) => editable.includes(field));

  const changedIn = (group: GroupName) => {
    const body: Record<string, unknown> = {};
    for (const field of GROUP_FIELDS[group]) {
      if (draft[field] !== baseline[field] && editable.includes(field)) body[field] = draft[field];
    }
    return body;
  };

  function save(group: GroupName) {
    const body = changedIn(group);
    if (Object.keys(body).length === 0) return;
    if (group === 'identity' && nameErrorKey) return;
    update.mutate({ id: vm.id, body }, { onSuccess: () => setBaseline(draft) });
  }

  return {
    draft,
    baseline,
    set,
    limits,
    zones,
    nameErrorKey,
    locked,
    changedIn,
    save,
    pending: update.isPending,
  };
}

export type VmSettingsForm = ReturnType<typeof useVmSettingsForm>;
