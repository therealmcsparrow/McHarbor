// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export interface StorageLocationRef {
  id: string;
  name: string;
  locationType: string;
  enabled: boolean;
}

/**
 * The host's storage destinations, for the backup destination picker.
 *
 * Read from the host's own endpoint rather than mirrored into this plugin:
 * destinations are shared with container backups and reports, and a second
 * copy would drift the moment one is added.
 */
export function useStorageLocations() {
  return useQuery({
    queryKey: ['storage-locations'],
    staleTime: 60_000,
    queryFn: async () => {
      const locations = assertSuccess(await api.get<StorageLocationRef[]>('/storage-locations'));
      return (locations ?? []).filter((loc) => loc.enabled);
    },
  });
}
