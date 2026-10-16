// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo } from 'react';
import { guestNameErrorKey } from '../guestName';
import { defaultDiskBus } from '../diskBus';
import { capabilityItem, useComputeCapability } from './useComputeCapability';
import type { CreateVMFields } from './useCreateVMFields';
import { useCreateVMQueries } from './useCreateVMQueries';
import {
  AGENT_DEFAULT_DATA_LOCATION,
  IMAGE_STEPS,
  MANUAL_STEPS,
  PRESETS,
  TEMPLATE_STEPS,
  guessOSFamily,
} from '../components/create-vm/model';

export function useCreateVMData(f: CreateVMFields) {
  const {
    step,
    environmentId,
    nameValue,
    source,
    imageId,
    setImageId,
    isoId,
    setIsoId,
    setPreset,
    setVcpus,
    setMemoryMb,
    setDiskGb,
    diskBus,
    poolId,
    setPoolId,
    diskMode,
    volumePath,
    setVolumePath,
    networkId,
    setNetworkId,
    setHostname,
    sshKeys,
    password,
    template,
  } = f;

  const { pools, networks, images, discs, ensurePool, pool, freeVolumes, volumesPending, volumesFailed } =
    useCreateVMQueries(environmentId, poolId);

  // The path a pool would be created at when this host has none. The pool
  // itself names the location once it exists, so this is display-only and the
  // server resolves it again on create — there is nothing to type here.
  const dataLocation = AGENT_DEFAULT_DATA_LOCATION;

  // Only images actually downloaded to this host can boot a guest.
  const readyImages = useMemo(() => (images ?? []).filter((image) => image.cache?.state === 'ready'), [images]);

  // Preselect the defaults so the common path is a few clicks.
  useEffect(() => {
    const first = pools?.find((entry) => entry.isDefault) ?? pools?.[0];
    if (!poolId && first) setPoolId(first.id);
  }, [pools, poolId, setPoolId]);

  useEffect(() => {
    const first = networks?.find((entry) => entry.isDefault) ?? networks?.[0];
    if (!networkId && first) setNetworkId(first.id);
  }, [networks, networkId, setNetworkId]);

  const readyDiscs = useMemo(() => (discs ?? []).filter((disc) => disc.cache?.state === 'ready'), [discs]);

  useEffect(() => {
    const first = readyImages[0];
    if (!imageId && first) setImageId(first.id);
  }, [readyImages, imageId, setImageId]);

  useEffect(() => {
    const first = readyDiscs[0];
    if (!isoId && first) setIsoId(first.id);
  }, [readyDiscs, isoId, setIsoId]);

  useEffect(() => {
    if (diskMode !== 'existing') return;
    if (!freeVolumes?.some((volume) => volume.volumePath === volumePath)) {
      setVolumePath(freeVolumes?.[0]?.volumePath ?? '');
    }
  }, [freeVolumes, diskMode, volumePath, setVolumePath]);

  // The host name follows the machine name unless the operator changes it.
  useEffect(() => {
    setHostname((current) => (current === '' ? nameValue : current));
  }, [nameValue, setHostname]);

  const applyPreset = (key: string) => {
    setPreset(key);
    const values = PRESETS[key];
    if (values) {
      setVcpus(values.vcpus);
      setMemoryMb(values.memoryMb);
      setDiskGb(values.diskGb);
    }
  };

  const nameErrorKey = nameValue.trim() === '' ? null : guestNameErrorKey(nameValue.trim());

  const { data: capability } = useComputeCapability(environmentId || undefined);
  const swtpm = capabilityItem(capability, 'swtpm');
  const tpmAvailable = swtpm?.present !== false;
  const secboot = capabilityItem(capability, 'ovmf_secboot');
  const secureBootAvailable = secboot?.present !== false;

  const hasPools = (pools?.length ?? 0) > 0;
  const network = networks?.find((entry) => entry.id === networkId);
  const canAdopt = Boolean(pool) && source !== 'image';
  const adopting = canAdopt && diskMode === 'existing';
  const volume = freeVolumes?.find((entry) => entry.volumePath === volumePath);
  const image = source === 'image' ? readyImages.find((entry) => entry.id === imageId) : undefined;
  const disc = source === 'iso' ? readyDiscs.find((entry) => entry.id === isoId) : undefined;

  // The bus follows the guest's family until the operator picks one, so a
  // Windows install gets a disk its installer can see without anyone having to
  // know that virtio needs a driver disc.
  const osFamily = guessOSFamily(image?.displayName ?? disc?.displayName);
  const effectiveDiskBus = diskBus || defaultDiskBus(osFamily);

  const steps = source === 'image' ? IMAGE_STEPS : source === 'template' ? TEMPLATE_STEPS : MANUAL_STEPS;
  const index = steps.indexOf(step);
  const sourceChosen =
    source === 'blank'
      ? true
      : source === 'image'
        ? Boolean(imageId)
        : source === 'template'
          ? Boolean(template)
          : Boolean(isoId);
  const canAdvance =
    step === 'source'
      ? Boolean(environmentId && nameValue.trim() && !nameErrorKey && sourceChosen)
      : step === 'storage'
        ? adopting
          ? Boolean(volumePath)
          : Boolean(hasPools ? poolId : dataLocation.trim())
        : step === 'network'
          ? Boolean(networkId)
          : step === 'setup'
            ? // A machine nobody can log into is not finished. One of the two is
              // required, and the SSH key is the one to prefer.
              Boolean(sshKeys.trim() || password)
            : true;

  return {
    pools,
    networks,
    dataLocation,
    ensurePool,
    pool,
    freeVolumes,
    volumesPending,
    volumesFailed,
    readyImages,
    readyDiscs,
    applyPreset,
    nameErrorKey,
    swtpm,
    tpmAvailable,
    secboot,
    secureBootAvailable,
    hasPools,
    network,
    canAdopt,
    adopting,
    volume,
    image,
    disc,
    osFamily,
    effectiveDiskBus,
    steps,
    index,
    canAdvance,
  };
}

export type CreateVMData = ReturnType<typeof useCreateVMData>;
