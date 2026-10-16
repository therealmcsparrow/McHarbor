// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { VM } from '../types';

export type StateFacet = 'all' | 'running' | 'stopped' | 'problem';

export const FACETS: readonly StateFacet[] = ['all', 'running', 'stopped', 'problem'];

export function matchesFacet(vm: VM, facet: StateFacet): boolean {
  switch (facet) {
    case 'running':
      return vm.state === 'running';
    case 'stopped':
      return vm.state === 'stopped' || vm.state === 'defined';
    case 'problem':
      return vm.state === 'crashed' || !vm.hostAvailable;
    default:
      return true;
  }
}
