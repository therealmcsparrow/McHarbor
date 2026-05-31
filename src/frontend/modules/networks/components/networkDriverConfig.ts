// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { DRIVER_OPTIONS, type Driver } from '@resources/constants/network-drivers';

export { DRIVER_OPTIONS, type Driver };

export interface DriverConfig {
  hasIpam: boolean;
  hasParent: boolean;
  hasMode: boolean;
  hasToggles: boolean;
  modes?: string[];
}

export const DRIVER_CONFIG: Record<Driver, DriverConfig> = {
  bridge:  { hasIpam: true,  hasParent: false, hasMode: false, hasToggles: true },
  host:    { hasIpam: false, hasParent: false, hasMode: false, hasToggles: false },
  overlay: { hasIpam: true,  hasParent: false, hasMode: false, hasToggles: true },
  macvlan: { hasIpam: true,  hasParent: true,  hasMode: true,  hasToggles: true, modes: ['bridge', 'vepa', 'passthru', 'private'] },
  ipvlan:  { hasIpam: true,  hasParent: true,  hasMode: true,  hasToggles: true, modes: ['l2', 'l3', 'l3s'] },
  none:    { hasIpam: false, hasParent: false, hasMode: false, hasToggles: false },
};
