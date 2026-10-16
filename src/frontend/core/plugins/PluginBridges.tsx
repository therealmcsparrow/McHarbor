// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactElement } from 'react';
import { ShellSlot } from '@resources/layout/shellSlots';
import { usePluginI18nEffect } from './pluginI18n';
import { PluginTopbarBridge } from './bridge-topbar';

/**
 * PluginBridges is a renderless component that activates the runtime
 * plugin bridges when mounted. It must be placed inside the AppLayout
 * so the bridges run only after the user is logged in.
 *
 * - The `pluginBridges` shell slot holds the bridges that reach into a
 *   feature module: the widget bridge registers plugin widgets as
 *   WidgetDefinitions, and the nodes bridge registers plugin nodes in the
 *   custom-node registry. The composition root fills it (shellWiring.tsx).
 * - usePluginI18nEffect merges plugin i18n bundles into the active
 *   language whenever the loader's i18n value transitions.
 * - PluginTopbarBridge mounts a button per plugin-contributed
 *   topbar entry into the host's `#header-slot` / `#header-right-slot`.
 */
export function PluginBridges(): ReactElement | null {
  return (
    <>
      <ShellSlot name="pluginBridges" />
      <PluginI18nBridge />
      <PluginTopbarBridge />
    </>
  );
}

function PluginI18nBridge(): null {
  usePluginI18nEffect();
  return null;
}
