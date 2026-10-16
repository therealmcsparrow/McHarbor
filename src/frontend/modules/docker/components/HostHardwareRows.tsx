// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// HostHardwareRows — GPUs, physical network adapters and attached devices
// inside the Docker Info → Resources section.
//
// The section used to answer "how much capacity" (CPUs, memory) and stop
// there, which leaves the operator's actual question — *what is in this
// machine* — unanswered: whether a GPU is present to pass through,
// which NIC the driver bound, what is plugged in. Docker's `/info` cannot
// say; the backend reads sysfs and this renders it.
//
// Scope is stated in the panel rather than assumed. The inventory belongs
// to the host running McHarbor, not to the selected environment, and a
// multi-host operator switching environments must not read a local card
// as the remote host's.

import { useTranslation } from 'react-i18next';
import type { HardwareDevice } from '@core/types/docker';
import { useHostHardware } from '@resources/hooks/useHostHardware';
import { InfoRow, formatBytes } from './docker-info-primitives';

/**
 * The detail line under a device name: everything identifying about it
 * that is not the name, joined into one muted string.
 *
 * Kind-specific because the useful facts differ — a disk's size says
 * nothing about a NIC, and a USB peripheral has no driver worth naming.
 */
function deviceDetail(device: HardwareDevice, removableLabel: string): string {
  const parts: string[] = [];
  if (device.interface) parts.push(device.interface);
  if (device.model) parts.push(device.model);
  if (device.driver) parts.push(device.driver);
  if (device.memoryTotal) parts.push(formatBytes(device.memoryTotal));
  if (typeof device.utilization === 'number') parts.push(`${Math.round(device.utilization)}%`);
  if (typeof device.temperature === 'number' && device.temperature > 0) {
    parts.push(`${Math.round(device.temperature)} °C`);
  }
  if (device.speedMbps) parts.push(`${device.speedMbps} Mbit/s`);
  if (device.capacity) parts.push(formatBytes(device.capacity));
  if (device.removable) parts.push(removableLabel);
  return parts.join(' · ');
}

type DeviceListProps = {
  devices: HardwareDevice[];
  emptyLabel: string;
  removableLabel: string;
};

function DeviceList({ devices, emptyLabel, removableLabel }: DeviceListProps) {
  if (devices.length === 0) {
    return <span className="text-muted-foreground">{emptyLabel}</span>;
  }

  return (
    <span className="flex flex-col items-end gap-1">
      {devices.map((device) => {
        const detail = deviceDetail(device, removableLabel);
        return (
          <span key={`${device.kind}-${device.id}`} className="flex flex-col items-end">
            <span>{device.name}</span>
            {detail && <span className="text-xs text-muted-foreground">{detail}</span>}
          </span>
        );
      })}
    </span>
  );
}

export function HostHardwareRows() {
  const { t } = useTranslation('docker');
  const { data: inventory, isLoading } = useHostHardware();

  const rows: Array<{ label: string; devices: HardwareDevice[] }> = [
    { label: t('info.gpu'), devices: inventory?.gpus ?? [] },
    { label: t('info.networkAdapters'), devices: inventory?.networkAdapters ?? [] },
    { label: t('info.attachedDevices'), devices: inventory?.attachedDevices ?? [] },
  ];

  // A platform with no sysfs cannot answer any of the three, and three
  // rows all reading "none detected" would be a lie by omission — it
  // reads as "this machine has no GPU" rather than "we could not look".
  if (!isLoading && inventory && !inventory.available) {
    return (
      <InfoRow
        label={t('info.hardware')}
        value={<span className="text-muted-foreground">{t('info.hardwareUnavailable')}</span>}
      />
    );
  }

  const emptyLabel = isLoading ? t('info.hardwareLoading') : t('info.hardwareNone');

  return (
    <>
      {rows.map((row) => (
        <InfoRow
          key={row.label}
          label={row.label}
          value={
            <DeviceList devices={row.devices} emptyLabel={emptyLabel} removableLabel={t('info.hardwareRemovable')} />
          }
        />
      ))}
      {/* Not an InfoRow: it is a caption for the three above, not a
          fourth fact, and giving it a label column would imply it is one. */}
      <p className="py-2 text-xs text-muted-foreground">{t('info.hardwareScope')}</p>
    </>
  );
}
