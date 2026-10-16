// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconPlugConnected, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { VM_PERM } from '../permissions';
import { useDetachDevice } from '../hooks/useVMs';
import type { VM } from '../types';

export function HardwareDevices({ vm, mutable, onAttach }: { vm: VM; mutable: boolean; onAttach: () => void }) {
  const { t } = useTranslation('vm');
  const detachDevice = useDetachDevice();
  const devices = vm.devices ?? [];

  return (
    <section>
      <div className="mb-2 flex items-center justify-between gap-2">
        <h3 className="flex items-center gap-2 font-medium text-foreground">
          <IconPlugConnected className="h-4 w-4" aria-hidden="true" />
          {t('hardware.devices')}
        </h3>
        {/*
          The attach button is gated on vm.privileged rather than hidden when
          the list is empty: passthrough is the one action here that can
          compromise the host, so the people who may do it should find it and
          everyone else should see why they cannot.
        */}
        <Button variant="outline" size="sm" permission={VM_PERM.PRIVILEGED} disabled={!mutable} onClick={onAttach}>
          {t('actions.attachDevice')}
        </Button>
      </div>

      {devices.length === 0 ? (
        <p className="rounded-lg border border-border p-3 text-sm text-muted-foreground">{t('hardware.deviceEmpty')}</p>
      ) : (
        <>
          <div className="flex items-start gap-2 rounded-md bg-warning/10 p-3">
            <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
            <p className="text-sm text-foreground">{t('hardware.attachDevice.warning')}</p>
          </div>
          <div className="mt-2 overflow-x-auto rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('hardware.deviceColumns.kind')}</TableHead>
                  <TableHead>{t('hardware.deviceColumns.address')}</TableHead>
                  <TableHead>{t('hardware.deviceColumns.description')}</TableHead>
                  <TableHead className="w-16" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {devices.map((device) => (
                  <TableRow key={device.id}>
                    <TableCell>
                      <Badge variant="outline">{device.kind}</Badge>
                    </TableCell>
                    <TableCell>
                      <code className="font-mono text-xs text-muted-foreground">{device.hostAddress}</code>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{device.description || '—'}</TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t('actions.delete')}
                        permission={VM_PERM.PRIVILEGED}
                        disabled={!mutable}
                        onClick={() =>
                          detachDevice.mutate({
                            id: vm.id,
                            deviceId: device.id,
                          })
                        }
                      >
                        <IconTrash className="h-4 w-4" aria-hidden="true" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </>
      )}
    </section>
  );
}
