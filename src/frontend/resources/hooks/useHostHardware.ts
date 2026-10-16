// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { HardwareInventory } from '@core/types/docker';
import { assertSuccess } from '@resources/utils/api-mutation';

/**
 * The device inventory of the host running McHarbor.
 *
 * Deliberately not keyed by environment: the backend reads sysfs on the
 * machine the process lives on, so the answer is the same whichever
 * environment is selected, and keying it by one would imply otherwise.
 * The UI states the scope for the same reason.
 *
 * No refetch interval — cards, adapters and disks change when someone
 * opens the case or plugs something in, not on a five-second cadence. The
 * usual query invalidation on remount is enough.
 */
export function useHostHardware(enabled = true) {
  return useQuery({
    queryKey: ['host-hardware'],
    queryFn: () => api.get<HardwareInventory>('/metrics/hardware').then(assertSuccess),
    staleTime: 5 * 60_000,
    enabled,
  });
}
