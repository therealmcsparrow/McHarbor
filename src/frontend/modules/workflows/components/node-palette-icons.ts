// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComponentType } from 'react';
import { ICON_MAP_A } from './node-palette-icons-a';
import { ICON_MAP_B } from './node-palette-icons-b';

export const ICON_MAP: Record<string, ComponentType<{ className?: string }>> = {
  ...ICON_MAP_A,
  ...ICON_MAP_B,
};
