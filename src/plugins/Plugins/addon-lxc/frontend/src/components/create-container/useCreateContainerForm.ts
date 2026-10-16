// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useHostEnvironment } from '../../hooks/useHostEnvironment';
import { useCreateContainer } from '../../hooks/useContainers';
import { useComputeNetworks } from '../../hooks/useComputeNetworks';
import { nicAttachment } from '../../utils/nicAttachment';
import type { AddressMode, CreateContainerInput, InitSystem } from '../../types';

export const STEPS = ['source', 'profile', 'resources', 'network', 'setup', 'summary'] as const;
export type Step = (typeof STEPS)[number];

const PRESETS: Record<string, { memoryMb: number; cpuMax: string }> = {
  small: { memoryMb: 512, cpuMax: '100000 100000' },
  medium: { memoryMb: 2048, cpuMax: '200000 100000' },
  large: { memoryMb: 8192, cpuMax: '400000 100000' },
};

export type CreateContainerForm = ReturnType<typeof useCreateContainerForm>;

/** Holds every field of the create wizard so the step components stay presentational. */
export function useCreateContainerForm(environments: { id: string; name: string }[], onCreated: (id: string) => void) {
  const create = useCreateContainer();

  const [step, setStep] = useState<Step>('source');
  // The host's topbar selection, not whatever sorted first — creating a guest
  // on a host the user had not chosen is the bug this fixes.
  const hostEnvironmentId = useHostEnvironment();
  const [environmentId, setEnvironmentId] = useState(hostEnvironmentId || environments[0]?.id || '');
  const [ctName, setCtName] = useState('');
  const [imageId, setImageId] = useState('');
  const [poolId, setPoolId] = useState('');
  const [poolBackend, setPoolBackend] = useState('');
  const [poolTarget, setPoolTarget] = useState('');
  const [initSystem, setInitSystem] = useState<InitSystem>('systemd');
  const [privileged, setPrivileged] = useState(false);
  const [nesting, setNesting] = useState(false);
  const [preset, setPreset] = useState('medium');
  const [memoryMb, setMemoryMb] = useState(2048);
  const [cpuMax, setCpuMax] = useState('200000 100000');
  const [pidsMax, setPidsMax] = useState(4096);
  const [networkId, setNetworkId] = useState('');
  const [addressMode, setAddressMode] = useState<AddressMode>('dhcp');
  const [staticIp, setStaticIp] = useState('');
  const [staticGw, setStaticGw] = useState('');
  const [staticIpv6, setStaticIpv6] = useState('');
  const [staticGw6, setStaticGw6] = useState('');
  const [hostname, setHostname] = useState('');
  const [sshKeys, setSshKeys] = useState('');
  const [password, setPassword] = useState('');
  const [startAfterCreate, setStartAfterCreate] = useState(false);

  const { data: networks } = useComputeNetworks(environmentId || undefined);
  const network = networks?.find((entry) => entry.id === networkId);

  const macvlanIpv6 = network?.mode === 'macvlan' && network.ipv6Enabled !== false;

  const index = STEPS.indexOf(step);
  const last = index === STEPS.length - 1;

  const applyPreset = (key: string) => {
    setPreset(key);
    const values = PRESETS[key];
    if (values) {
      setMemoryMb(values.memoryMb);
      setCpuMax(values.cpuMax);
    }
  };

  // Without an SSH key or a password the only way in is the console. Not
  // refused — a container that runs one service and is never logged into is a
  // perfectly reasonable thing — but said out loud before it is created.
  const noWayIn = !sshKeys.trim() && !password;

  const submit = () => {
    const body: CreateContainerInput = {
      environmentId,
      name: ctName,
      imageId: imageId || undefined,
      poolId,
      poolBackend,
      poolTarget,
      initSystem,
      privileged,
      nesting,
      limits: {
        memoryMaxBytes: memoryMb * 1024 * 1024,
        cpuMax,
        pidsMax,
        swapMaxBytes: 0,
      },
      nics: network
        ? [
            {
              networkId: network.id,
              ...nicAttachment(network),
              addressMode,
              staticIp: addressMode === 'static' ? staticIp : undefined,
              staticGw: addressMode === 'static' ? staticGw : undefined,
              staticIpv6: macvlanIpv6 && staticIpv6.trim() ? staticIpv6.trim() : undefined,
              staticGw6: macvlanIpv6 && staticIpv6.trim() && staticGw6.trim() ? staticGw6.trim() : undefined,
            },
          ]
        : undefined,
      hostname: hostname || ctName,
      sshKeys: sshKeys
        .split('\n')
        .map((key) => key.trim())
        .filter(Boolean),
      password: password || undefined,
      startAfterCreate,
    };

    create.mutate(body, { onSuccess: (created) => created && onCreated(created.id) });
  };

  return {
    create,
    step,
    setStep,
    index,
    last,
    environments,
    environmentId,
    setEnvironmentId,
    ctName,
    setCtName,
    imageId,
    setImageId,
    poolId,
    setPoolId,
    poolBackend,
    setPoolBackend,
    poolTarget,
    setPoolTarget,
    initSystem,
    setInitSystem,
    privileged,
    setPrivileged,
    nesting,
    setNesting,
    preset,
    setPreset,
    applyPreset,
    memoryMb,
    setMemoryMb,
    cpuMax,
    setCpuMax,
    pidsMax,
    setPidsMax,
    networks,
    network,
    networkId,
    setNetworkId,
    addressMode,
    setAddressMode,
    staticIp,
    setStaticIp,
    staticGw,
    setStaticGw,
    staticIpv6,
    setStaticIpv6,
    staticGw6,
    setStaticGw6,
    macvlanIpv6,
    hostname,
    setHostname,
    sshKeys,
    setSshKeys,
    password,
    setPassword,
    noWayIn,
    startAfterCreate,
    setStartAfterCreate,
    submit,
  };
}
