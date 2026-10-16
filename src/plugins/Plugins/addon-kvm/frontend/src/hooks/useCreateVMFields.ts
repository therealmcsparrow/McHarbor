// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useHostEnvironment } from './useHostEnvironment';
import { useVMs } from './useVMs';
import type { DiskMode, Source, Step } from '../components/create-vm/model';

export function useCreateVMFields(environments: { id: string; name: string }[]) {
  const { data: allTemplates } = useVMs(undefined, true);

  const [step, setStep] = useState<Step>('source');
  // The host's topbar selection, not whatever sorted first — creating a guest
  // on a host the user had not chosen is the bug this fixes.
  const hostEnvironmentId = useHostEnvironment();
  const [environmentId, setEnvironmentId] = useState(hostEnvironmentId || environments[0]?.id || '');
  const [nameValue, setName] = useState('');
  const [source, setSource] = useState<Source>('image');
  const [imageId, setImageId] = useState('');
  const [isoId, setIsoId] = useState('');
  const [preset, setPreset] = useState('medium');
  const [vcpus, setVcpus] = useState(2);
  const [memoryMb, setMemoryMb] = useState(4096);
  const [diskGb, setDiskGb] = useState(40);
  const [firmware, setFirmware] = useState<'uefi' | 'bios'>('uefi');
  const [secureBoot, setSecureBoot] = useState(false);
  const [diskBus, setDiskBus] = useState('');
  const [tpmEnabled, setTpmEnabled] = useState(false);
  const [poolId, setPoolId] = useState('');
  const [diskMode, setDiskMode] = useState<DiskMode>('new');
  const [volumePath, setVolumePath] = useState('');
  const [networkId, setNetworkId] = useState('');
  const [hostname, setHostname] = useState('');
  const [username, setUsername] = useState('mcharbor');
  const [sshKeys, setSshKeys] = useState('');
  const [password, setPassword] = useState('');
  const [passwordFromVault, setPasswordFromVault] = useState(false);
  const [passwordSecret, setPasswordSecret] = useState('');
  const [sshKeySecrets, setSshKeySecrets] = useState<string[]>([]);
  const [installAgent, setInstallAgent] = useState(true);
  const [autostart, setAutostart] = useState(false);
  const [pickedTemplateId, setTemplateId] = useState('');
  const [linked, setLinked] = useState(true);

  const hostTemplates = useMemo(
    () => (allTemplates ?? []).filter((entry) => entry.environmentId === environmentId),
    [allTemplates, environmentId],
  );
  const templateId = hostTemplates.some((entry) => entry.id === pickedTemplateId)
    ? pickedTemplateId
    : (hostTemplates[0]?.id ?? '');
  const template = hostTemplates.find((entry) => entry.id === templateId);

  return {
    step,
    setStep,
    environmentId,
    setEnvironmentId,
    nameValue,
    setName,
    source,
    setSource,
    imageId,
    setImageId,
    isoId,
    setIsoId,
    preset,
    setPreset,
    vcpus,
    setVcpus,
    memoryMb,
    setMemoryMb,
    diskGb,
    setDiskGb,
    firmware,
    setFirmware,
    secureBoot,
    setSecureBoot,
    diskBus,
    setDiskBus,
    tpmEnabled,
    setTpmEnabled,
    poolId,
    setPoolId,
    diskMode,
    setDiskMode,
    volumePath,
    setVolumePath,
    networkId,
    setNetworkId,
    hostname,
    setHostname,
    username,
    setUsername,
    sshKeys,
    setSshKeys,
    password,
    setPassword,
    passwordFromVault,
    setPasswordFromVault,
    passwordSecret,
    setPasswordSecret,
    sshKeySecrets,
    setSshKeySecrets,
    installAgent,
    setInstallAgent,
    autostart,
    setAutostart,
    templateId,
    setTemplateId,
    linked,
    setLinked,
    hostTemplates,
    template,
  };
}

export type CreateVMFields = ReturnType<typeof useCreateVMFields>;
