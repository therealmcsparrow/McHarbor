// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

/** A vault entry as the host's secrets list returns it: never a value. */
export type VaultSecret = {
  id: string;
  name: string;
  kind?: string;
  scope: string;
};

/**
 * The secrets visible in the guest's environment, for the create wizard.
 * Disabled without secrets.view: the host route would refuse anyway, and the
 * wizard shows the missing key instead of an empty list.
 */
export function useVaultSecrets(environmentId: string, enabled: boolean) {
  return useQuery({
    queryKey: ['kvm', 'vault-secrets', environmentId],
    enabled: enabled && Boolean(environmentId),
    // The list route is paginated ({ items, total, … }); one page of the
    // API's maximum is every secret a picker can sensibly show.
    queryFn: async () =>
      assertSuccess(await api.get<{ items: VaultSecret[] }>('/secrets', { env: environmentId, per_page: '1000' }))
        ?.items ?? [],
  });
}
