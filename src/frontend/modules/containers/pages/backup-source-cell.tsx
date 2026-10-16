// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconBox, IconArchive } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { SOURCE_CONTAINER } from './backup-run-rows';
import type { SourceTagged } from '../hooks/useSourceLabel';

export function RunSourceCell({ row, label }: { row: SourceTagged; label: string }) {
  const Icon = row.sourceKind === SOURCE_CONTAINER ? IconBox : IconArchive;
  return (
    <Badge variant="outline" className="gap-1 whitespace-nowrap">
      <Icon className="size-3.5 opacity-70" aria-hidden="true" />
      {label}
    </Badge>
  );
}
