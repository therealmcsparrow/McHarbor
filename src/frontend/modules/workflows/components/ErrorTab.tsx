// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';

export type ErrorEntry = {
  id: string;
  timestamp: string;
  nodeId?: string;
  nodeLabel?: string;
  message: string;
  stack?: string;
};

type ErrorTabProps = {
  errors: ErrorEntry[];
  onClear: () => void;
};

export function ErrorTab({ errors, onClear }: ErrorTabProps) {
  const { t } = useTranslation('common');

  if (errors.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-4">
        <IconAlertTriangle className="size-8 text-muted-foreground/20" />
        <p className="mt-3 text-xs text-muted-foreground">{t('workflows.noErrors')}</p>
        <p className="mt-1 text-[10px] text-muted-foreground/60">
          {t('workflows.noErrorsDescription')}
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col">
      <div className="flex items-center justify-between border-b border-border px-4 py-2">
        <span className="text-[10px] text-muted-foreground">{t('workflows.entries', { count: errors.length })}</span>
        <Button variant="link" onClick={onClear} className="h-auto p-0 text-[10px]">
          {t('workflows.clear')}
        </Button>
      </div>
      <div className="flex-1 overflow-y-auto">
      {errors.map((err) => (
        <div key={err.id} className="border-b border-border px-4 py-2.5">
          <div className="flex items-center gap-2">
            <IconAlertTriangle className="size-3 shrink-0 text-red-400" />
            <span className="text-[10px] text-muted-foreground/60 font-mono">{err.timestamp}</span>
            {err.nodeLabel && (
              <span className="truncate text-[10px] text-foreground">{err.nodeLabel}</span>
            )}
          </div>
          <p className="mt-1 text-[10px] text-red-400">{err.message}</p>
          {err.stack && (
            <pre className="mt-1 overflow-x-auto whitespace-pre-wrap text-[9px] text-muted-foreground/60 font-mono">{err.stack}</pre>
          )}
        </div>
      ))}
      </div>
    </div>
  );
}

