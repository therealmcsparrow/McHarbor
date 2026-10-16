// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconEye } from '@tabler/icons-react';
import type { ContainerProcesses } from '@core/types/docker';
import { Button } from '@resources/components/ui/Button';
import type { SelectedProcessContext } from './ProcessContextDrawer';

type ProcessesMobileListProps = {
  processes: ContainerProcesses;
  onSelect: (context: SelectedProcessContext) => void;
};

export function ProcessesMobileList({ processes, onSelect }: ProcessesMobileListProps) {
  const { t } = useTranslation('containers');
  return (
    <div className="space-y-3 md:hidden">
      {processes.Processes.map((process) => (
        <div key={process.join('\u0000')} className="rounded-md border border-border bg-muted/30 p-3">
          <div className="flex items-start justify-between gap-3">
            <dl className="min-w-0 space-y-2">
              {process.slice(0, 3).map((value, index) => (
                <div key={`${processes.Titles[index] ?? 'column'}-${value}`}>
                  <dt className="text-xs text-muted-foreground">
                    {processes.Titles[index] ?? t('processes.column', { number: index + 1 })}
                  </dt>
                  <dd className="font-mono text-xs break-all text-foreground">{value}</dd>
                </div>
              ))}
            </dl>
            <Button
              variant="ghost"
              size="icon"
              className="shrink-0"
              onClick={() =>
                onSelect({
                  kind: 'process',
                  titles: processes.Titles,
                  values: process,
                })
              }
              aria-label={t('processes.viewProcessDetails')}
              aria-haspopup="dialog"
            >
              <IconEye className="size-4" />
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
}
