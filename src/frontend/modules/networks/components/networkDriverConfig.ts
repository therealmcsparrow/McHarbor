// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type Driver = 'bridge' | 'host' | 'overlay' | 'macvlan' | 'ipvlan' | 'none';

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

export const DRIVER_OPTIONS = [
  { value: 'bridge', label: 'bridge' },
  { value: 'host', label: 'host' },
  { value: 'overlay', label: 'overlay' },
  { value: 'macvlan', label: 'macvlan' },
  { value: 'ipvlan', label: 'ipvlan' },
  { value: 'none', label: 'none' },
] as const;
