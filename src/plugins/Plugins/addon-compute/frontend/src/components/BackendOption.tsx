// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconBolt } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import type { PoolBackendInfo } from '../types';

/** One selectable backend, with its recommendation or its remediation. */
export function BackendOption({
  entry,
  selected,
  onSelect,
}: {
  entry: PoolBackendInfo;
  selected: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation('compute');

  return (
    <Button
      type="button"
      variant="ghost"
      size="lg"
      onClick={entry.available ? onSelect : undefined}
      disabled={!entry.available}
      aria-pressed={selected}
      className={cn(
        'block h-auto w-full rounded-lg border p-3 text-left font-normal whitespace-normal transition-colors',
        selected ? 'border-primary bg-primary/5' : 'border-border bg-card',
        entry.available ? 'hover:border-primary/60' : 'cursor-not-allowed opacity-60',
      )}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium text-foreground">{t(`pools.backend.${entry.backend}`)}</span>
        {entry.instant && entry.available && (
          <Badge variant="success">
            <IconBolt className="h-3 w-3" aria-hidden="true" />
            {t('pools.create.recommended')}
          </Badge>
        )}
        {!entry.available && <Badge variant="outline">{t('pools.create.unavailable')}</Badge>}
      </div>

      {entry.detail && <p className="mt-1 text-xs text-muted-foreground">{entry.detail}</p>}

      {entry.remediation && (
        <code className="mt-2 block overflow-x-auto rounded bg-muted px-2 py-1 font-mono text-xs text-foreground">
          {entry.remediation}
        </code>
      )}
    </Button>
  );
}

/**
 * Instant backends first, available before unavailable.
 *
 * This ordering is the recommendation — the top item is the one the operator
 * should pick, and it is at the top for a reason that is stated on it.
 */
export function sortBackends(entries: PoolBackendInfo[]): PoolBackendInfo[] {
  return [...entries].sort((a, b) => {
    if (a.available !== b.available) return a.available ? -1 : 1;
    if (Boolean(a.instant) !== Boolean(b.instant)) return a.instant ? -1 : 1;
    return a.backend.localeCompare(b.backend);
  });
}
