// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDisc, IconPlayerEject } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { GuestAgentDiscButton } from './GuestAgentDiscButton';
import { VM_PERM } from '../permissions';
import { useEjectMedia } from '../hooks/useVMs';
import type { Disk, VM } from '../types';

export function HardwareMedia({
  vm,
  media,
  changeable,
  live,
  onAttach,
}: {
  vm: VM;
  media: Disk[];
  /** Stopped, or running on a host that can swap discs live. */
  changeable: boolean;
  live: boolean;
  onAttach: () => void;
}) {
  const { t } = useTranslation('vm');
  const eject = useEjectMedia();

  return (
    <section>
      <div className="mb-2 flex items-center justify-between gap-2">
        <h3 className="flex items-center gap-2 font-medium text-foreground">
          <IconDisc className="h-4 w-4" aria-hidden="true" />
          {t('hardware.media.title')}
        </h3>
        <div className="flex items-center gap-2">
          <GuestAgentDiscButton vm={vm} appearance="labelled" disabled={!changeable} live={live} />
          <Button variant="outline" size="sm" permission={VM_PERM.MANAGE} disabled={!changeable} onClick={onAttach}>
            {t('actions.attachMedia')}
          </Button>
        </div>
      </div>

      {media.length === 0 ? (
        <p className="rounded-lg border border-border p-3 text-sm text-muted-foreground">{t('hardware.media.empty')}</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('hardware.media.columns.slot')}</TableHead>
                <TableHead>{t('hardware.media.columns.source')}</TableHead>
                <TableHead className="w-16" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {media.map((disc) => (
                <TableRow key={disc.id}>
                  <TableCell>{disc.slot}</TableCell>
                  <TableCell>
                    {disc.volumePath ? (
                      <code className="font-mono text-xs text-muted-foreground">{disc.volumePath}</code>
                    ) : (
                      <span className="text-sm text-muted-foreground">{t('hardware.emptyDrive')}</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label={t('actions.ejectMedia')}
                      permission={VM_PERM.MANAGE}
                      // A running guest keeps an emptied drive, so there is
                      // nothing left to eject from it until it stops.
                      disabled={!changeable || (live && !disc.volumePath)}
                      onClick={() => eject.mutate({ id: vm.id, diskId: disc.id })}
                    >
                      <IconPlayerEject className="h-4 w-4" aria-hidden="true" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </section>
  );
}
