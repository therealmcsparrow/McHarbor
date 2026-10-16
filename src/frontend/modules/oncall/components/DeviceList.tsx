// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { formatDateCustom } from '@resources/utils/format';
import { IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { usePushDevices, useRevokePushDevice, type PushDevice } from '../hooks/usePushDevice';
import { QueryState } from './QueryState';

function deviceName(device: PushDevice): string {
  return device.label || device.userAgent || device.id;
}

export function DeviceList() {
  const { t } = useTranslation('oncall');
  const devices = usePushDevices();
  const revoke = useRevokePushDevice();
  const [target, setTarget] = useState<PushDevice | null>(null);
  const format = (value: string) => formatDateCustom(value, { dateStyle: 'short', timeStyle: 'short' });
  const list = devices.data ?? [];

  return (
    <div className="flex flex-col gap-2 rounded-lg border border-border bg-card p-4">
      <h2 className="font-medium">{t('device.devicesTitle')}</h2>
      <QueryState
        isLoading={devices.isLoading}
        error={devices.error}
        onRetry={() => void devices.refetch()}
        isEmpty={list.length === 0}
        emptyText={t('device.noDevices')}
      >
        <ul className="divide-y divide-border">
          {list.map((device) => (
            <li key={device.id} className="flex min-h-14 items-center gap-2 py-1">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{deviceName(device)}</p>
                <p className="text-xs text-muted-foreground">
                  {device.lastSuccessAt
                    ? t('device.lastSeen', { time: format(device.lastSuccessAt) })
                    : t('device.neverDelivered')}
                  {device.failureCount > 0 && ` · ${t('device.failing', { count: device.failureCount })}`}
                </p>
              </div>
              <Button
                variant="ghost"
                size="icon"
                className="size-11"
                aria-label={t('device.revokeLabel', { name: deviceName(device) })}
                onClick={() => setTarget(device)}
              >
                <IconTrash className="size-5" aria-hidden="true" />
              </Button>
            </li>
          ))}
        </ul>
      </QueryState>
      <MutationConfirmDialog
        open={target !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) setTarget(null);
        }}
        title={t('device.revokeTitle', { name: target ? deviceName(target) : '' })}
        description={t('device.revokeDescription')}
        confirmLabel={t('device.revoke')}
        mutation={revoke}
        variables={target?.id ?? ''}
      />
    </div>
  );
}
