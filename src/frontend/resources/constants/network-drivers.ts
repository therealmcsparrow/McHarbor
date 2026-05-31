// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type Driver = 'bridge' | 'host' | 'overlay' | 'macvlan' | 'ipvlan' | 'none';

export const DRIVER_OPTIONS = [
  { value: 'bridge', label: 'bridge' },
  { value: 'host', label: 'host' },
  { value: 'overlay', label: 'overlay' },
  { value: 'macvlan', label: 'macvlan' },
  { value: 'ipvlan', label: 'ipvlan' },
  { value: 'none', label: 'none' },
] as const;
