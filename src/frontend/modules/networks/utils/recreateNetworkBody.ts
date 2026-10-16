// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { CreateNetworkRequest } from '@resources/hooks/useNetworks';
import type { NetworkEditData } from '../hooks/useNetworkDetailState';
import { hostLinkRequestOf } from './hostLinkForm';

/**
 * The create body for recreating an edited network. The host link comes from the original labels,
 * not the editable ones: the server owns those labels and writes them again from `hostLink`.
 */
export function recreateNetworkBody(editData: NetworkEditData): CreateNetworkRequest {
  return {
    name: editData.name,
    driver: editData.driver,
    internal: editData.internal,
    attachable: editData.attachable,
    enableIPv6: editData.enableIPv6,
    ipam: {
      Driver: editData.ipamDriver,
      Config: editData.ipamConfig
        .filter((config) => config.subnet)
        .map((config) => ({
          Subnet: config.subnet,
          Gateway: config.gateway || undefined,
          IPRange: config.ipRange || undefined,
          AuxiliaryAddresses:
            config.auxAddresses && Object.keys(config.auxAddresses).length > 0 ? config.auxAddresses : undefined,
        })),
    },
    options: Object.fromEntries(
      editData.options.filter((option) => option.key).map((option) => [option.key, option.value]),
    ),
    labels: Object.fromEntries(editData.labels.filter((label) => label.key).map((label) => [label.key, label.value])),
    hostLink: editData.driver === 'macvlan' ? hostLinkRequestOf(editData.hostLink, editData.enableIPv6) : undefined,
  };
}
