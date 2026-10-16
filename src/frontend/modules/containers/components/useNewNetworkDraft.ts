// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import type { NetworkInfo } from '@core/types/docker';
import { DRIVER_CONFIG, type Driver } from '@resources/constants/network-drivers';
import { useCreateNetwork } from '@resources/hooks/useNetworks';

function createdNetworkName(data: unknown): string | undefined {
  if (typeof data !== 'object' || data === null || !('name' in data)) return undefined;
  return typeof data.name === 'string' ? data.name : undefined;
}

export function useNewNetworkDraft(onNetworkCreated: (network: NetworkInfo) => void) {
  const createNetwork = useCreateNetwork();
  const [newName, setNewName] = useState('');
  const [newDriver, setNewDriver] = useState<Driver>('bridge');
  const [newSubnet, setNewSubnet] = useState('');
  const [newGateway, setNewGateway] = useState('');
  const [newInternal, setNewInternal] = useState(false);
  const [newAttachable, setNewAttachable] = useState(false);

  function submit() {
    if (!newName.trim()) return;
    const cfg = DRIVER_CONFIG[newDriver];
    const options: Record<string, string> = {};
    if (cfg.hasMode && cfg.modes && cfg.modes.length > 0) {
      const key = newDriver === 'macvlan' ? 'macvlan_mode' : 'ipvlan_mode';
      options[key] = cfg.modes[0] ?? '';
    }
    const ipamConfig: { Subnet: string; Gateway: string }[] =
      newSubnet.trim() || newGateway.trim() ? [{ Subnet: newSubnet.trim(), Gateway: newGateway.trim() }] : [];

    createNetwork.mutate(
      {
        name: newName.trim(),
        driver: newDriver,
        internal: cfg.hasToggles && newInternal,
        attachable: cfg.hasToggles && newAttachable,
        ipam: cfg.hasIpam && ipamConfig.length > 0 ? { Config: ipamConfig } : undefined,
        options: Object.keys(options).length > 0 ? options : undefined,
      },
      {
        onSuccess: (data) => {
          const itemName = createdNetworkName(data) ?? newName.trim();
          setNewName('');
          setNewSubnet('');
          setNewGateway('');
          setNewInternal(false);
          setNewAttachable(false);
          onNetworkCreated({ Name: itemName } as NetworkInfo);
        },
      },
    );
  }

  return {
    createNetwork,
    newName,
    setNewName,
    newDriver,
    setNewDriver,
    newSubnet,
    setNewSubnet,
    newGateway,
    setNewGateway,
    newInternal,
    setNewInternal,
    newAttachable,
    setNewAttachable,
    submit,
  };
}

export type NewNetworkDraft = ReturnType<typeof useNewNetworkDraft>;
