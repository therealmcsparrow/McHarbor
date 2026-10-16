// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { HardwareDisks } from './HardwareDisks';
import { HardwareMedia } from './HardwareMedia';
import { HardwareNICs } from './HardwareNICs';
import { HardwareDevices } from './HardwareDevices';
import { AddDiskDialog } from './AddDiskDialog';
import { AddNICDialog } from './AddNICDialog';
import { AttachDeviceDialog } from './AttachDeviceDialog';
import { AttachMediaDialog } from './AttachMediaDialog';
import { LiveResizePanel } from './LiveResizePanel';
import { SharesPanel } from './SharesPanel';
import { useLiveHardware } from '../hooks/useLiveHardware';
import type { VM } from '../types';

type OpenDialog = 'disk' | 'media' | 'nic' | 'device' | null;

/**
 * A guest's disks, installer discs, interfaces and passed-through host devices.
 *
 * On a running guest whose host can hot-plug, data disks, discs and NICs change
 * live; the boot disk, disk types, boot order and host devices still need it
 * stopped. The server decides what a given guest can take, and says why when
 * it cannot, so a control is enabled on the feature and not on a guess.
 */
export function HardwarePanel({ vm }: { vm: VM }) {
  const { t } = useTranslation('vm');
  const [isOpen, setOpen] = useState<OpenDialog>(null);

  const { mutable, live, changeable } = useLiveHardware(vm);

  const { disks, media } = useMemo(() => {
    const all = vm.disks ?? [];
    return {
      disks: all.filter((disk) => !disk.isCdrom),
      media: all.filter((disk) => disk.isCdrom),
    };
  }, [vm.disks]);

  return (
    <div className="space-y-6">
      {!mutable && vm.hostAvailable && (
        <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">
          {live ? t('hardware.liveChanges') : t('hardware.mustBeStopped')}
        </p>
      )}

      <LiveResizePanel vm={vm} />

      <HardwareDisks vm={vm} disks={disks} mutable={mutable} live={live} onAdd={() => setOpen('disk')} />
      <HardwareMedia vm={vm} media={media} changeable={changeable} live={live} onAttach={() => setOpen('media')} />
      <HardwareNICs vm={vm} mutable={mutable} live={live} onAdd={() => setOpen('nic')} />
      <HardwareDevices vm={vm} mutable={mutable} onAttach={() => setOpen('device')} />
      <SharesPanel vm={vm} />

      {isOpen === 'disk' && <AddDiskDialog vm={vm} onClose={() => setOpen(null)} />}
      {isOpen === 'media' && <AttachMediaDialog vm={vm} onClose={() => setOpen(null)} />}
      {isOpen === 'nic' && <AddNICDialog vm={vm} onClose={() => setOpen(null)} />}
      {isOpen === 'device' && <AttachDeviceDialog vm={vm} onClose={() => setOpen(null)} />}
    </div>
  );
}
