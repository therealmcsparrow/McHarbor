// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback } from 'react';

/** Returns a stable callback ref that focuses its element once, when it mounts. */
export function useFocusOnMount<T extends HTMLElement>() {
  return useCallback((el: T | null) => {
    el?.focus();
  }, []);
}
