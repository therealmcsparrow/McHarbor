// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { createContext, useContext, type ReactNode } from 'react';

const PageActionsSlotContext = createContext<HTMLElement | null>(null);

export function PageActionsSlotProvider({ element, children }: { element: HTMLElement | null; children: ReactNode }) {
  return <PageActionsSlotContext.Provider value={element}>{children}</PageActionsSlotContext.Provider>;
}

export function usePageActionsSlot(): HTMLElement | null {
  return useContext(PageActionsSlotContext);
}
