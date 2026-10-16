// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { supportsFeature, useQEMUCapability, VM_FEATURE } from './useQEMUCapability';
import type { VM } from '../types';

/**
 * Which hardware changes a guest takes right now. `mutable` is the stopped
 * guest every control has always worked on; `live` is a running guest on a
 * host whose agent can hot-plug, where disks, discs and NICs change too.
 */
export function useLiveHardware(vm: VM) {
  const { data: capability } = useQEMUCapability(vm.environmentId);
  const mutable = vm.hostAvailable && vm.state !== 'running';
  const live = vm.hostAvailable && vm.state === 'running' && supportsFeature(capability, VM_FEATURE.HOTPLUG);
  return { mutable, live, changeable: mutable || live };
}
