// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { cn } from '@resources/utils/cn';
import { api } from '@core/api/client';

type LinkOutputInfo = {
  workflowId: string;
  workflowName: string;
  nodeId: string;
  name: string;
};

interface LinkOutputSelectProps {
  value: string;
  onChange: (v: string) => void;
}

export function LinkOutputSelect({ value, onChange }: LinkOutputSelectProps) {
  const { t } = useTranslation('common');
  const [open, setOpen] = useState(false);

  const { data: outputs } = useQuery({
    queryKey: ['workflow-link-outputs'],
    queryFn: () =>
      api.get<LinkOutputInfo[]>('/workflows/link-outputs').then((r) => r.data ?? []),
    staleTime: 30_000,
  });

  // Group by workflow
  const grouped = (outputs ?? []).reduce<Record<string, LinkOutputInfo[]>>((acc, item) => {
    const key = item.workflowId;
    if (!acc[key]) acc[key] = [];
    acc[key].push(item);
    return acc;
  }, {});

  const selected = outputs?.find((o) => `${o.workflowId}:${o.nodeId}` === value);
  const displayName = selected
    ? `${selected.name || 'Unnamed'} (${selected.workflowName})`
    : value || t('workflows.selectLinkOutput');

  return (
    <div className="relative">
      {/* Raw <button> kept: custom select trigger styled as form input with chevron doesn't fit Button's API */}
      <button
        type="button"
        aria-label={t('workflows.selectLinkOutput')}
        onClick={() => setOpen(!open)}
        className={cn(
          'flex h-8 w-full items-center justify-between rounded-md border border-input bg-card px-2 text-xs ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
          !value && 'text-muted-foreground',
        )}
      >
        <span className="truncate">{displayName}</span>
        <svg className="size-3 shrink-0 text-muted-foreground" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5">
          <path d="M3 4.5L6 7.5L9 4.5" />
        </svg>
      </button>
      {open && (
        <div className="absolute left-0 right-0 top-full z-50 mt-1 max-h-48 overflow-y-auto rounded-md border border-border bg-popover py-1 shadow-xl">
          {!outputs || outputs.length === 0 ? (
            <p className="px-3 py-2 text-xs text-muted-foreground">{t('workflows.noLinkOutputsFound')}</p>
          ) : (
            Object.entries(grouped).map(([wfId, items]) => {
              const groupName = items[0]?.workflowName ?? wfId;
              return (
              <div key={wfId}>
                <p className="px-3 py-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                  {groupName}
                </p>
                {items.map((item) => {
                  const compositeKey = `${item.workflowId}:${item.nodeId}`;
                  const isSelected = compositeKey === value;
                  return (
                    /* Raw <button> kept: dropdown list item with multi-line content doesn't fit Button's API */
                    <button
                      key={compositeKey}
                      type="button"
                      onClick={() => { onChange(compositeKey); setOpen(false); }}
                      className={cn(
                        'flex w-full flex-col gap-0.5 px-3 py-1.5 text-left transition-colors hover:bg-muted/50',
                        isSelected && 'bg-muted/50',
                      )}
                    >
                      <span className="text-xs font-semibold text-foreground">{item.name || 'Unnamed'}</span>
                    </button>
                  );
                })}
              </div>
              );
            })
          )}
        </div>
      )}
    </div>
  );
}
