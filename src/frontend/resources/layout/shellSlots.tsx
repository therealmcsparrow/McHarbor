// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComponentType } from 'react';

export type ShellSlotName = 'afterHeader' | 'banners' | 'pluginBridges';

type ShellSlotEntry = { id: string; component: ComponentType };

const slots: Record<ShellSlotName, ShellSlotEntry[]> = {
  afterHeader: [],
  banners: [],
  pluginBridges: [],
};

/** Lets the composition root plug a feature component into a shell region, so the shell never imports a feature module. */
export function registerShellSlot(slot: ShellSlotName, id: string, component: ComponentType): void {
  if (slots[slot].some((entry) => entry.id === id)) return;
  slots[slot].push({ id, component });
}

/** Renders every component registered for the region, in registration order. */
export function ShellSlot({ name: slot }: { name: ShellSlotName }) {
  return (
    <>
      {slots[slot].map(({ id, component: Registered }) => (
        <Registered key={id} />
      ))}
    </>
  );
}
