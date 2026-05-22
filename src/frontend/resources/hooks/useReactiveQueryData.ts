// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useSyncExternalStore } from 'react';
import { type QueryKey, useQueryClient } from '@tanstack/react-query';

export function useReactiveQueryData<T>(queryKey: QueryKey) {
  const queryClient = useQueryClient();

  return useSyncExternalStore(
    (onStoreChange) => queryClient.getQueryCache().subscribe(() => onStoreChange()),
    () => queryClient.getQueryData<T>(queryKey),
    () => queryClient.getQueryData<T>(queryKey),
  );
}
