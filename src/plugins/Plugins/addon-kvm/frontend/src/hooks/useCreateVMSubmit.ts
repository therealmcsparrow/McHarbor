// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCloneVM, useCreateVM, useVMLifecycle } from './useVMs';
import type { CreateVMInput } from '../types';
import { nicAttachment } from '../utils/nicAttachment';
import type { CreateVMData } from './useCreateVMData';
import type { CreateVMFields } from './useCreateVMFields';

export function useCreateVMSubmit(f: CreateVMFields, d: CreateVMData, onCreated: (id: string) => void) {
  const create = useCreateVM();
  const cloneVM = useCloneVM();
  const lifecycle = useVMLifecycle();
  const {
    environmentId,
    nameValue,
    source,
    vcpus,
    memoryMb,
    diskGb,
    firmware,
    secureBoot,
    tpmEnabled,
    hostname,
    username,
    sshKeys,
    password,
    passwordFromVault,
    passwordSecret,
    sshKeySecrets,
    installAgent,
    autostart,
    linked,
    template,
  } = f;
  const {
    ensurePool,
    pool,
    tpmAvailable,
    secureBootAvailable,
    network,
    adopting,
    volume,
    image,
    disc,
    osFamily,
    effectiveDiskBus,
  } = d;

  const submitFromTemplate = () => {
    if (!template) return;
    cloneVM.mutate(
      { id: template.id, name: nameValue.trim(), linked },
      {
        onSuccess: (vm) => {
          if (autostart) lifecycle.mutate({ id: vm.id, action: 'start' });
          onCreated(vm.id);
        },
      },
    );
  };

  const submit = async () => {
    if (source === 'template') {
      submitFromTemplate();
      return;
    }
    if (!network) return;

    let storage = pool;
    if (!storage) {
      // Empty on purpose: the compute plugin resolves the Drives location
      // itself, so the pool cannot land somewhere other than what was shown.
      storage = await ensurePool.mutateAsync('');
    }
    if (!storage) return;
    if (adopting && !volume) return;

    const body: CreateVMInput = {
      environmentId,
      name: nameValue.trim(),
      guestOsFamily: osFamily,
      vcpus,
      memoryMb,
      firmware,
      secureBoot: firmware === 'uefi' && secureBoot && secureBootAvailable,
      tpmEnabled: firmware === 'uefi' && tpmEnabled && tpmAvailable,
      poolId: storage.id,
      // Copied through because this plugin cannot read compute's tables and
      // the agent acts on a target rather than an id.
      poolBackend: storage.backend,
      poolTarget: storage.target,
      diskSizeGb: adopting ? 0 : diskGb,
      diskBus: effectiveDiskBus,
      diskVolumePath: volume?.volumePath,
      diskFormat: volume?.format,
      diskSizeBytes: volume?.sizeBytes,
      networkId: network.id,
      ...nicAttachment(network),
      imageId: image?.id,
      imageLocalPath: image?.cache?.localPath,
      isoPaths: disc?.cache?.localPath ? [disc.cache.localPath] : undefined,
      autostart,
      cloudInit: {
        hostname: hostname.trim() || nameValue.trim(),
        defaultUser: username.trim() || 'mcharbor',
        sshKeys: sshKeys
          .split('\n')
          .map((line) => line.trim())
          .filter(Boolean),
        // Sent once, hashed on receipt, never stored or returned in this form.
        // A vault secret is sent by name only; the server reads it as this user.
        password: passwordFromVault ? undefined : password || undefined,
        passwordSecret: passwordFromVault ? passwordSecret || undefined : undefined,
        sshKeySecrets: sshKeySecrets.length > 0 ? sshKeySecrets : undefined,
        networkMode: 'dhcp',
        installGuestAgent: installAgent,
      },
    };

    if (source !== 'image') delete body.cloudInit;

    create.mutate(body, { onSuccess: (vm) => onCreated(vm.id) });
  };

  return { create, cloneVM, submit };
}
