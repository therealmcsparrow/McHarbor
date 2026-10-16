// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useComputeNetworks, useComputePools } from './useComputeResources';
import { dismissImportJob, isImportRunning, useImportJob } from '../importJob';

export function useImportForm(environments: { id: string; name: string }[], onImported: (vmId: string) => void) {
  const job = useImportJob();
  const running = isImportRunning(job.phase);
  const uploaded = job.record;
  const found = uploaded?.inspection;

  const [envId, setEnvId] = useState(environments[0]?.id ?? '');
  const [nameValue, setName] = useState('');
  const [poolId, setPoolId] = useState('');
  const [networkId, setNetworkId] = useState('');
  const [vcpus, setVcpus] = useState(0);
  const [memoryMb, setMemoryMb] = useState(0);
  const [firmware, setFirmware] = useState('');
  const [guestOs, setGuestOs] = useState('');
  const [secureBoot, setSecureBoot] = useState(false);
  const [tpmEnabled, setTpmEnabled] = useState(false);
  const [autostart, setAutostart] = useState(false);

  const pools = useComputePools(envId);
  const networks = useComputeNetworks(envId);

  const pool = useMemo(() => (pools.data ?? []).find((entry) => entry.id === poolId), [pools.data, poolId]);
  const network = useMemo(
    () => (networks.data ?? []).find((entry) => entry.id === networkId),
    [networks.data, networkId],
  );

  const [seenUpload, setSeenUpload] = useState<object | null>(null);
  if (seenUpload !== (uploaded ?? null)) {
    setSeenUpload(uploaded ?? null);
    if (uploaded) setName((current) => current || uploaded.fileName.replace(/\.[^.]+$/, ''));
  }

  const [seenInspection, setSeenInspection] = useState<object | null>(null);
  if (seenInspection !== (found ?? null)) {
    setSeenInspection(found ?? null);
    if (found) {
      const { name: foundName, vcpus: foundVcpus, memoryMb: foundMemory } = found;
      if (foundName) setName((current) => current || foundName);
      if (foundVcpus) setVcpus((current) => current || foundVcpus);
      if (foundMemory) setMemoryMb((current) => current || foundMemory);
      setFirmware((current) => current || found.firmware || 'bios');
      setGuestOs((current) => current || found.guestOsFamily || 'linux');
    }
  }

  if (firmware !== 'uefi' && secureBoot) setSecureBoot(false);

  useEffect(() => {
    if (job.phase !== 'done' || !job.vmId) return;
    const id = job.vmId;
    dismissImportJob();
    onImported(id);
  }, [job.phase, job.vmId, onImported]);

  const step = found ? 'review' : uploaded ? 'inspect' : 'upload';
  const hostName = environments.find((env) => env.id === envId)?.name ?? envId;

  const uploadedEnvId = uploaded?.environmentId;
  const [seenEnvId, setSeenEnvId] = useState<string | undefined>(undefined);
  if (seenEnvId !== uploadedEnvId) {
    setSeenEnvId(uploadedEnvId);
    if (uploadedEnvId) setEnvId(uploadedEnvId);
  }

  return {
    job,
    running,
    uploaded,
    found,
    step,
    hostName,
    envId,
    setEnvId,
    nameValue,
    setName,
    poolId,
    setPoolId,
    networkId,
    setNetworkId,
    vcpus,
    setVcpus,
    memoryMb,
    setMemoryMb,
    firmware,
    setFirmware,
    guestOs,
    setGuestOs,
    secureBoot,
    setSecureBoot,
    tpmEnabled,
    setTpmEnabled,
    autostart,
    setAutostart,
    pools,
    networks,
    pool,
    network,
  };
}

export type ImportForm = ReturnType<typeof useImportForm>;
