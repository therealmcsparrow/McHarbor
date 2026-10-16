// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconFileImport } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { useImports } from '../hooks/useImports';
import { useWidgetContext } from './mount';

/**
 * Third-party disks waiting to become guests.
 *
 * An import that stops halfway leaves tens of gigabytes on the host and no
 * guest to show for it, and the only place that was previously visible was the
 * import dialog someone had already closed.
 */
export function ImportsWidget() {
  const { t } = useTranslation('vm');
  const { navigate } = useWidgetContext();
  const { data, isLoading } = useImports();

  if (isLoading) return <Skeleton className="h-full w-full" />;

  const imports = data?.imports ?? [];
  const busy = imports.filter((entry) => entry.state === 'inspecting' || entry.state === 'converting');
  const failed = imports.filter((entry) => entry.state === 'failed');

  if (imports.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
        <IconFileImport className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
        <p className="text-sm text-muted-foreground">{t('widgets.vmImports.empty')}</p>
      </div>
    );
  }

  return (
    <Button
      type="button"
      variant="ghost"
      size="lg"
      onClick={() => navigate('/plugins/addon-kvm')}
      className="flex h-full w-full flex-col items-stretch justify-start gap-2 rounded p-1 text-left font-normal whitespace-normal hover:bg-muted/50"
    >
      <div className="flex items-baseline gap-2">
        <span className="text-2xl font-semibold text-foreground">{imports.length}</span>
        <span className="text-sm text-muted-foreground">{t('widgets.vmImports.waiting', { count: busy.length })}</span>
      </div>

      {failed.length > 0 && (
        <Badge variant="destructive" className="w-fit">
          {t('widgets.vmImports.failed', { count: failed.length })}
        </Badge>
      )}

      <div className="min-h-0 flex-1 space-y-1 overflow-y-auto">
        {imports.slice(0, 5).map((entry) => (
          <div key={entry.id} className="flex items-center justify-between gap-2">
            <span className="min-w-0 truncate text-sm text-foreground">{entry.fileName}</span>
            <span className="shrink-0 text-xs text-muted-foreground">{t(`import.state.${entry.state}`)}</span>
          </div>
        ))}
      </div>
    </Button>
  );
}
