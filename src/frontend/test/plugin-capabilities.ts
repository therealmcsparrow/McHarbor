// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act } from '@testing-library/react';
import { usePluginLoader, type PluginCapability } from '@core/plugins/loader';

/**
 * Installs `caps` in the real loader store, each under a plugin that is enabled and
 * loaded unless named in `notLoaded` — the same join the host's capability hooks make.
 */
export function givePluginCapabilities(caps: PluginCapability[], notLoaded: string[] = []): void {
  const slugs = [...new Set(caps.map((c) => c.plugin))];
  act(() => {
    usePluginLoader.setState({
      capabilities: caps,
      installed: slugs.map((slug) => ({
        slug,
        version: '1',
        enabled: true,
        loaded: !notLoaded.includes(slug),
      })),
    });
  });
}

export function clearPluginCapabilities(): void {
  givePluginCapabilities([]);
}
