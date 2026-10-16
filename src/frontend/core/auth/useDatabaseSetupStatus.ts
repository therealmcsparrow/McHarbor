// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

/** The part of GET /database/setup/status the pre-login redirect needs. */
export type DatabaseSetupGate = {
  configured: boolean;
  envPinned: boolean;
};

/** Shares its query key with the settings database module's hook, so the first-run screens read one cache entry. */
export function useDatabaseSetupStatus(enabled = true) {
  return useQuery({
    queryKey: ['database', 'setup', 'status'],
    queryFn: () => api.get<DatabaseSetupGate>('/database/setup/status').then(assertSuccess),
    enabled,
    retry: false,
  });
}
