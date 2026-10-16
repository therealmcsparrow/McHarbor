// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import {
  EMPTY_GPU_REQUEST,
  type CreateContainerPayload,
  type GpuRequest,
  type NetworkChoice,
  type PortBinding,
  type RestartPolicy,
  type VolumeMount,
} from '../hooks/useCreateContainer';
import type { DeviceMappingEntry } from '../types/edit-form';
import { DEFAULT_NETWORK, splitEnv, type CreateContainerTab, type EnvRow } from './createContainerFormHelpers';
import { parseComposeService } from './createContainerCompose';
import { parseDockerRun } from './createContainerParsers';

export function useCreateContainerForm(isOpen: boolean) {
  const [tab, setTab] = useState<CreateContainerTab>('form');

  const [image, setImage] = useState('');
  const [entryName, setName] = useState('');
  const [ports, setPorts] = useState<PortBinding[]>([]);
  const [env, setEnv] = useState<EnvRow[]>([]);
  const [secretFileMode, setSecretFileMode] = useState(false);
  const [volumes, setVolumes] = useState<VolumeMount[]>([]);
  const [network, setNetwork] = useState<NetworkChoice>(DEFAULT_NETWORK);
  const [restart, setRestart] = useState<RestartPolicy>('no');
  const [autoRemove, setAutoRemove] = useState(false);
  const [autoStart, setAutoStart] = useState(true);
  const [pullImage, setPullImage] = useState(true);
  const [tty, setTty] = useState(false);
  const [stdin, setStdin] = useState(false);
  const [gpu, setGpu] = useState<GpuRequest>(EMPTY_GPU_REQUEST);
  const [devices, setDevices] = useState<DeviceMappingEntry[]>([]);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [cmd, setCmd] = useState('');
  const [entrypoint, setEntrypoint] = useState('');
  const [workingDir, setWorkingDir] = useState('');
  const [user, setUser] = useState('');
  const [hostname, setHostname] = useState('');

  const [composeText, setComposeText] = useState('');
  const [runText, setRunText] = useState('');
  const [parseWarnings, setParseWarnings] = useState<string[]>([]);
  const [parseError, setParseError] = useState<string | null>(null);

  const [wasOpen, setWasOpen] = useState(isOpen);
  if (wasOpen !== isOpen) {
    setWasOpen(isOpen);
    if (!isOpen) {
      setImage('');
      setName('');
      setPorts([]);
      setEnv([]);
      setSecretFileMode(false);
      setVolumes([]);
      setNetwork(DEFAULT_NETWORK);
      setRestart('no');
      setAutoRemove(false);
      setAutoStart(true);
      setPullImage(true);
      setTty(false);
      setStdin(false);
      setGpu(EMPTY_GPU_REQUEST);
      setDevices([]);
      setAdvancedOpen(false);
      setCmd('');
      setEntrypoint('');
      setWorkingDir('');
      setUser('');
      setHostname('');
      setComposeText('');
      setRunText('');
      setParseWarnings([]);
      setParseError(null);
      setTab('form');
    }
  }

  function applyPayload(payload: CreateContainerPayload) {
    setImage(payload.image ?? '');
    setName(payload.name ?? '');
    setPorts(payload.ports ?? []);
    setEnv((payload.env ?? []).map((line) => splitEnv(line)));
    setVolumes(payload.volumes ?? []);
    setNetwork(payload.network ?? DEFAULT_NETWORK);
    setRestart(payload.restart ?? 'no');
    setAutoRemove(!!payload.autoRemove);
    setTty(!!payload.tty);
    setStdin(!!payload.openStdin);
    setCmd((payload.cmd ?? []).join(' '));
    setEntrypoint((payload.entrypoint ?? []).join(' '));
    setWorkingDir(payload.workingDir ?? '');
    setUser(payload.user ?? '');
    setHostname(payload.hostname ?? '');
  }

  function applyParsed(parse: () => { payload: CreateContainerPayload; warnings: string[] }) {
    setParseError(null);
    setParseWarnings([]);
    try {
      const { payload, warnings } = parse();
      applyPayload(payload);
      setParseWarnings(warnings);
      setTab('form');
    } catch (err) {
      setParseError(err instanceof Error ? err.message : String(err));
    }
  }

  return {
    tab,
    setTab,
    image,
    setImage,
    entryName,
    setName,
    ports,
    setPorts,
    env,
    setEnv,
    secretFileMode,
    setSecretFileMode,
    volumes,
    setVolumes,
    network,
    setNetwork,
    restart,
    setRestart,
    autoRemove,
    setAutoRemove,
    autoStart,
    setAutoStart,
    pullImage,
    setPullImage,
    tty,
    setTty,
    stdin,
    setStdin,
    gpu,
    setGpu,
    devices,
    setDevices,
    advancedOpen,
    setAdvancedOpen,
    cmd,
    setCmd,
    entrypoint,
    setEntrypoint,
    workingDir,
    setWorkingDir,
    user,
    setUser,
    hostname,
    setHostname,
    composeText,
    setComposeText,
    runText,
    setRunText,
    parseWarnings,
    parseError,
    applyFromCompose: () => applyParsed(() => parseComposeService(composeText)),
    applyFromRun: () => applyParsed(() => parseDockerRun(runText)),
  };
}

export type CreateContainerForm = ReturnType<typeof useCreateContainerForm>;
