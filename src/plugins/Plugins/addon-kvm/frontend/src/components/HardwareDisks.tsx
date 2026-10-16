// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useState } from 'react';
import { IconArrowsMaximize, IconGauge, IconPlus, IconStethoscope } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Switch } from '@resources/components/ui/Switch';
import { Badge } from '@resources/components/ui/Badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { VM_PERM } from '../permissions';
import { useChangeDiskBus, useUpdateDiskFlags } from '../hooks/useVMs';
import { DISK_BUSES } from '../diskBus';
import { isBootDisk } from '../bootDisk';
import { DiskDeleteButton } from './DiskDeleteButton';
import { formatBytes } from '../format';
import { DiskLimitsDialog } from './DiskLimitsDialog';
import { DiskInspectDialog } from './DiskInspectDialog';
import { DiskResizeDialog } from './DiskResizeDialog';
import type { Disk, VM } from '../types';

const COW_LABEL = 'CoW';

export function HardwareDisks({
  vm,
  disks,
  mutable,
  live,
  onAdd,
}: {
  vm: VM;
  disks: Disk[];
  /** Stopped: every control works. */
  mutable: boolean;
  /** Running with hot-plug: disks come and go, their settings do not. */
  live: boolean;
  onAdd: () => void;
}) {
  const { t } = useTranslation('vm');
  const changeBus = useChangeDiskBus();
  const [limitsFor, setLimitsFor] = useState<Disk | null>(null);
  const [inspectFor, setInspectFor] = useState<Disk | null>(null);
  const [resizeFor, setResizeFor] = useState<Disk | null>(null);
  const updateFlags = useUpdateDiskFlags();

  return (
    <section>
      <div className="mb-2 flex items-center justify-between gap-2">
        <h3 className="font-medium text-foreground">{t('hardware.disks')}</h3>
        <Button variant="outline" size="sm" permission={VM_PERM.MANAGE} disabled={!mutable && !live} onClick={onAdd}>
          <IconPlus className="mr-1.5 h-4 w-4" aria-hidden="true" />
          {t('actions.addDisk')}
        </Button>
      </div>
      <div className="overflow-x-auto rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('hardware.diskColumns.slot')}</TableHead>
              <TableHead>{t('hardware.diskColumns.bus')}</TableHead>
              <TableHead>{t('hardware.diskColumns.size')}</TableHead>
              <TableHead>{t('hardware.diskColumns.pool')}</TableHead>
              <TableHead>{t('hardware.diskColumns.bootOrder')}</TableHead>
              <TableHead>{t('hardware.diskColumns.shared')}</TableHead>
              <TableHead>{t('hardware.diskColumns.limits')}</TableHead>
              <TableHead className="w-40" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {disks.map((disk) => (
              <TableRow key={disk.id}>
                <TableCell>
                  <div className="flex items-center gap-2">
                    <span>{disk.slot}</span>
                    {disk.isBoot && <Badge variant="secondary">{t('hardware.diskColumns.boot')}</Badge>}
                    {/*
                      A disk that is copy-on-write over an image cannot outlive
                      its backing file, which is worth knowing before anyone
                      evicts that image from the host.
                    */}
                    {disk.backingPath && (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Badge variant="outline">{COW_LABEL}</Badge>
                        </TooltipTrigger>
                        <TooltipContent>{disk.backingPath}</TooltipContent>
                      </Tooltip>
                    )}
                  </div>
                </TableCell>
                <TableCell>
                  {disk.isCdrom ? (
                    <span className="text-muted-foreground">{disk.bus}</span>
                  ) : (
                    <Select
                      value={disk.bus}
                      ariaLabel={t('hardware.diskColumns.bus')}
                      disabled={!mutable || changeBus.isPending}
                      options={DISK_BUSES.map((bus) => ({ value: bus, label: bus }))}
                      onChange={(bus) => changeBus.mutate({ id: vm.id, diskId: disk.id, bus })}
                    />
                  )}
                </TableCell>
                <TableCell>{formatBytes(disk.sizeBytes)}</TableCell>
                <TableCell className="text-muted-foreground">{disk.poolBackend}</TableCell>
                <TableCell>
                  {/*
                    Zero means "work it out from the boot flag", which is how
                    every guest created before the order was selectable still
                    boots. The placeholder says so rather than showing a 0 that
                    reads like first place.
                  */}
                  <NumberInput
                    value={disk.bootIndex ?? 0}
                    min={0}
                    max={99}
                    className="w-20"
                    aria-label={t('hardware.diskColumns.bootOrder')}
                    disabled={!mutable || updateFlags.isPending}
                    onChange={(value) => updateFlags.mutate({ id: vm.id, diskId: disk.id, bootIndex: value })}
                  />
                </TableCell>
                <TableCell>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span>
                        <Switch
                          checked={disk.shareRw ?? false}
                          aria-label={t('hardware.diskColumns.shared')}
                          disabled={!mutable || disk.isCdrom || updateFlags.isPending}
                          onCheckedChange={(value) =>
                            updateFlags.mutate({ id: vm.id, diskId: disk.id, shareRw: value })
                          }
                        />
                      </span>
                    </TooltipTrigger>
                    <TooltipContent className="max-w-[320px]">{t('hardware.diskColumns.sharedHint')}</TooltipContent>
                  </Tooltip>
                </TableCell>
                <TableCell className="text-muted-foreground">{describeLimits(disk, t)}</TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t('limits.open')}
                    permission={VM_PERM.MANAGE}
                    onClick={() => setLimitsFor(disk)}
                  >
                    <IconGauge className="h-4 w-4" aria-hidden="true" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t('inspect.open')}
                    permission={VM_PERM.MANAGE}
                    onClick={() => setInspectFor(disk)}
                  >
                    <IconStethoscope className="h-4 w-4" aria-hidden="true" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t('actions.resizeDisk')}
                    permission={VM_PERM.MANAGE}
                    disabled={disk.isCdrom || (!mutable && !live)}
                    onClick={() => setResizeFor(disk)}
                  >
                    <IconArrowsMaximize className="h-4 w-4" aria-hidden="true" />
                  </Button>
                  <DiskDeleteButton
                    vm={vm}
                    disk={disk}
                    changeable={mutable || live}
                    live={live}
                    bootDisk={isBootDisk(disks, disk)}
                  />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      {limitsFor && <DiskLimitsDialog vm={vm} disk={limitsFor} onClose={() => setLimitsFor(null)} />}
      {resizeFor && <DiskResizeDialog vm={vm} disk={resizeFor} onClose={() => setResizeFor(null)} />}
      {inspectFor && <DiskInspectDialog vm={vm} disk={inspectFor} onClose={() => setInspectFor(null)} />}
    </section>
  );
}

/**
 * A disk's limits in one readable phrase.
 *
 * Readable first, the raw numbers never: "up to 200 MB/s" is what an operator
 * checks at a glance, and four separate byte counts in a cell is what they
 * have to decode.
 */
function describeLimits(disk: Disk, t: (key: string, options?: Record<string, unknown>) => string) {
  const MB = 1024 * 1024;
  const parts: string[] = [];
  const bps = Math.max(disk.readBps ?? 0, disk.writeBps ?? 0);
  const iops = Math.max(disk.readIops ?? 0, disk.writeIops ?? 0);
  if (bps > 0) parts.push(t('limits.upToMbps', { value: Math.round(bps / MB) }));
  if (iops > 0) parts.push(t('limits.upToIops', { value: iops }));
  return parts.length > 0 ? parts.join(' · ') : t('limits.unlimited');
}
