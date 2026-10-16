// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useDemoStatus } from '@resources/hooks/useDemoMode';
import type { SettingsTabId } from './settings-tab-groups';

/**
 * Sections the instance does not offer, removed from the rail rather than
 * disabled: unlike a permission, there is nothing a viewer could ask for.
 *
 * Demo mode is offered only when a `demo_mode` file sits in McHarbor's root.
 * A demo that is already running keeps its section, so it can always be left.
 * Until the status loads the section stays hidden rather than flashing in.
 */
export function useHiddenSettingsTabs(): ReadonlySet<SettingsTabId> {
  const { data } = useDemoStatus();
  const offered = Boolean(data?.available || data?.active);
  return useMemo(() => new Set<SettingsTabId>(offered ? [] : ['demo']), [offered]);
}
