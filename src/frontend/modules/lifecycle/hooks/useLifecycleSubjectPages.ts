// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { usePluginCapabilities } from '@core/plugins/capabilities';

/**
 * The page each plugin-owned subject type links to, from the `lifecycle-subject`
 * capabilities of the loaded plugins. A type no plugin claims is absent, so its
 * events render without a destination rather than linking to a dead page.
 */
export function useLifecycleSubjectPages(): Record<string, string> {
  const caps = usePluginCapabilities('lifecycle-subject');
  return useMemo(() => {
    const pages: Record<string, string> = {};
    for (const c of caps) {
      if (c.subjectType && c.page && !(c.subjectType in pages)) pages[c.subjectType] = c.page;
    }
    return pages;
  }, [caps]);
}
