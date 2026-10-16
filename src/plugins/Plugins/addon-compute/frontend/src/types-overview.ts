// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Capability, Network, Pool, UnavailableReason } from './types';

export interface OverviewEntry {
  environmentId: string;
  name: string;
  computeEnabled: boolean;
  available: boolean;
  unavailableReason?: UnavailableReason;
  capability?: Capability;
  pools?: Pool[];
  networks?: Network[];
  cachedImages: number;
  setupComplete: boolean;
  sharedLibrary?: boolean;
}

export interface Overview {
  environments: OverviewEntry[];
}

export interface ApiEnvelope<T> {
  success: boolean;
  data?: T;
  error?: string;
  code?: string;
}

export interface Locations {
  id?: string;
  scopeId: string;
  name?: string;
  vmPath?: string;
  diskPath?: string;
  imagePath?: string;
  createdAt?: string;
  updatedAt?: string;
}

export type LocationSource = 'global' | 'environment' | 'agent';

export interface EffectiveLocations {
  environmentId: string;
  name?: string;
  vmPath?: string;
  diskPath?: string;
  imagePath?: string;
  vmSource: LocationSource;
  diskSource: LocationSource;
  imageSource: LocationSource;
  applied: boolean;
  lastError?: string;
}

export interface EnvironmentRef {
  id: string;
  name?: string;
}

export interface LocationsView {
  global: Locations;
  overrides: Locations[];
  available: EnvironmentRef[];
  agentDefaults: Locations;
}

export interface EnvironmentLocationsView {
  stored: Locations;
  effective: EffectiveLocations;
  global: Locations;
  agentDefaults: Locations;
}
