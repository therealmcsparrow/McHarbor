// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDeviceDesktop } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { Badge } from '@resources/components/ui/Badge';
import { useVMs } from '../hooks/useVMs';
import { useWidgetContext } from './mount';

/**
 * How many guests are running, stopped, or need attention.
 *
 * "Needs attention" is the useful number and is deliberately not just
 * "stopped": a guest someone shut down on purpose is fine. What matters is a
 * crash, or a host nobody can reach.
 */
export function SummaryWidget() {
  const { t } = useTranslation('vm');
  const { navigate } = useWidgetContext();
  const { data, isLoading } = useVMs();

  if (isLoading) return <Skeleton className="h-full w-full" />;

  const guests = (data ?? []).filter((vm) => !vm.isTemplate);
  const running = guests.filter((vm) => vm.state === 'running').length;
  const attention = guests.filter((vm) => vm.state === 'crashed' || !vm.hostAvailable);

  if (guests.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
        <IconDeviceDesktop className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
        <p className="text-sm text-muted-foreground">{t('list.empty.title')}</p>
      </div>
    );
  }

  return (
    <Button
      type="button"
      variant="ghost"
      size="lg"
      onClick={() => navigate('/plugins/addon-kvm')}
      className="flex h-full w-full flex-col items-stretch justify-start gap-3 rounded p-1 text-left font-normal whitespace-normal hover:bg-muted/50"
    >
      <div className="flex items-baseline gap-2">
        <span className="text-2xl font-semibold text-foreground">{running}</span>
        <span className="text-sm text-muted-foreground">
          / {guests.length} {t('state.running').toLowerCase()}
        </span>
      </div>

      {attention.length > 0 && (
        <div className="min-h-0 flex-1 space-y-1 overflow-y-auto">
          {attention.slice(0, 5).map((vm) => (
            <div key={vm.id} className="flex items-center justify-between gap-2">
              <span className="min-w-0 truncate text-sm text-foreground">{vm.name}</span>
              <Badge variant={vm.state === 'crashed' ? 'destructive' : 'warning'}>{t(`state.${vm.state}`)}</Badge>
            </div>
          ))}
        </div>
      )}
    </Button>
  );
}
