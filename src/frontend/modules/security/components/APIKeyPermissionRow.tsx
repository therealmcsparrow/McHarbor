// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconLock, IconChevronRight } from '@tabler/icons-react';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { cn } from '@resources/utils/cn';
import type { CategoryRow } from './apiKeyPermissionRows';

/**
 * One category of the permission matrix. Collapsed it shows the granted
 * actions as chips; expanded it explains each action, which role granted it,
 * and whether the owner's rights actually allow it.
 */
export function APIKeyPermissionRow({ row }: { row: CategoryRow }) {
  const { t } = useTranslation('security');
  const [isOpen, setOpen] = useState(false);
  const panelId = `perm-cat-${row.category}`;

  return (
    <li className="border-b border-border last:border-b-0">
      <PlainButton
        onClick={() => setOpen((previous) => !previous)}
        aria-expanded={isOpen}
        aria-controls={panelId}
        className="flex w-full items-center gap-3 px-3 py-2 text-left transition-colors hover:bg-muted/50"
      >
        <IconChevronRight
          className={cn('size-4 shrink-0 text-muted-foreground transition-transform', isOpen && 'rotate-90')}
          aria-hidden="true"
        />
        <span className="min-w-0 flex-1 truncate text-sm text-foreground">{row.label}</span>
        <span className="flex shrink-0 flex-wrap justify-end gap-1">
          {row.actions.map((cell) => (
            <span
              key={cell.key}
              title={`${cell.key}${cell.description ? ` — ${cell.description}` : ''}`}
              className={cn(
                'flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px]',
                cell.effective ? 'bg-muted text-muted-foreground' : 'bg-warning/10 text-warning',
              )}
            >
              {!cell.effective && <IconLock className="size-3" aria-hidden="true" />}
              {cell.action}
            </span>
          ))}
        </span>
      </PlainButton>

      {isOpen && (
        <ul id={panelId} className="space-y-3 border-t border-border bg-muted/30 px-3 py-3">
          {row.actions.map((cell) => (
            <li key={cell.key} className="min-w-0">
              <div className="flex flex-wrap items-baseline gap-2">
                <span className="text-sm font-medium text-foreground">{cell.action}</span>
                <code className="font-mono text-[11px] text-muted-foreground">{cell.key}</code>
                {!cell.effective && (
                  <span className="flex items-center gap-1 text-[11px] text-warning">
                    <IconLock className="size-3" aria-hidden="true" />
                    {t('apiKeys.detail.matrixBlockedLabel')}
                  </span>
                )}
              </div>
              {cell.description && <p className="mt-0.5 text-sm text-muted-foreground">{cell.description}</p>}
              <p className="mt-0.5 text-[11px] text-muted-foreground">
                {t('apiKeys.detail.matrixGrantedBy')}:{' '}
                {cell.sources.length === 0
                  ? t('apiKeys.detail.matrixNoSource')
                  : cell.sources
                      .map((source) => (source.direct ? t('apiKeys.detail.matrixDirect') : source.label))
                      .join(', ')}
              </p>
              {!cell.effective && (
                <p className="mt-0.5 text-[11px] text-warning">{t('apiKeys.detail.matrixBlockedHint')}</p>
              )}
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}
