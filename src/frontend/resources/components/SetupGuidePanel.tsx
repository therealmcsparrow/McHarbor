// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';

/** Separates a plugin-rendered setup guide from the form above it; the dialog body scrolls, so a long guide never overflows. */
export function SetupGuidePanel({ children }: { children: ReactNode }) {
  return <section className="mt-4 rounded-lg border border-border bg-muted/30 p-4">{children}</section>;
}
