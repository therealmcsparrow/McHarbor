// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  IconAlertTriangle,
  IconCpu2,
  IconDeviceDesktop,
  IconKeyboard,
  IconNetwork,
  IconPencil,
  IconPlug,
  IconUsb,
  IconVideo,
  IconVolume,
} from '@tabler/icons-react';
import type { SelectOption } from '@resources/components/ui/Select';
import type { HostDevice, HostDeviceCategory } from '../../hooks/useHostDevices';

type Translator = (key: string, options?: Record<string, unknown>) => string;

export const MANUAL_DEVICE_VALUE = '__manual__';

export const DEVICE_CATEGORY_ORDER: HostDeviceCategory[] = [
  'serial',
  'gpu',
  'audio',
  'video',
  'input',
  'network',
  'usb',
  'system',
  'block',
];

const CATEGORY_ICONS: Record<HostDeviceCategory, SelectOption['icon']> = {
  serial: IconPlug,
  gpu: IconDeviceDesktop,
  audio: IconVolume,
  video: IconVideo,
  input: IconKeyboard,
  network: IconNetwork,
  usb: IconUsb,
  system: IconCpu2,
  block: IconAlertTriangle,
};

export const CGROUP_PERMISSION_PRESETS = ['rwm', 'rw', 'r', 'm'] as const;

export function deviceValue(device: HostDevice): string {
  return device.stablePath || device.path;
}

export function findHostDevice(devices: HostDevice[], value: string): HostDevice | undefined {
  if (!value) return undefined;
  return devices.find(
    (device) => device.path === value || device.stablePath === value || (device.links ?? []).includes(value),
  );
}

function describeDevice(device: HostDevice, t: Translator): string {
  const parts: string[] = [];
  if (device.stablePath) parts.push(device.stablePath);
  parts.push(
    t('resources.hostDevices.node', {
      type: t(`resources.hostDevices.type.${device.type}`),
      major: device.major,
      minor: device.minor,
    }),
  );
  if (device.dangerous) parts.push(t('resources.hostDevices.blockBadge'));
  return parts.join(' · ');
}

export function buildDeviceOptions(devices: HostDevice[], t: Translator): SelectOption[] {
  const rank = (device: HostDevice) => DEVICE_CATEGORY_ORDER.indexOf(device.category);
  const sorted = [...devices].sort((a, b) => rank(a) - rank(b) || a.path.localeCompare(b.path));
  const options: SelectOption[] = sorted.map((device) => ({
    value: deviceValue(device),
    label: device.label ? `${device.label} (${device.path})` : device.path,
    description: describeDevice(device, t),
    group: t(`resources.hostDevices.category.${device.category}`),
    icon: CATEGORY_ICONS[device.category],
    iconClassName: device.dangerous ? 'text-warning' : undefined,
  }));
  options.push({
    value: MANUAL_DEVICE_VALUE,
    label: t('resources.hostDevices.enterManually'),
    description: t('resources.hostDevices.enterManuallyHint'),
    icon: IconPencil,
  });
  return options;
}

export function buildPermissionOptions(current: string, t: Translator): SelectOption[] {
  const options: SelectOption[] = CGROUP_PERMISSION_PRESETS.map((preset) => ({
    value: preset,
    label: preset,
    description: t(`resources.hostDevices.permissions.${preset}`),
  }));
  const trimmed = current.trim();
  if (trimmed && !(CGROUP_PERMISSION_PRESETS as readonly string[]).includes(trimmed)) {
    options.push({
      value: trimmed,
      label: trimmed,
      description: t('resources.hostDevices.permissions.custom'),
    });
  }
  return options;
}

export function freeContainerPath(path: string, taken: string[]): string {
  const used = new Set(taken.map((p) => p.trim()).filter(Boolean));
  if (!used.has(path)) return path;
  const match = /^(.*?)(\d+)$/.exec(path);
  const stem = match ? match[1] : path;
  let n = match ? Number(match[2]) + 1 : 1;
  while (used.has(`${stem}${n}`)) n += 1;
  return `${stem}${n}`;
}

export function nextContainerPath(
  currentContainerPath: string,
  previousHostPath: string,
  previousDevice: HostDevice | undefined,
  nextHostPath: string,
  nextDevice: HostDevice | undefined,
): string {
  const untouched =
    currentContainerPath === '' ||
    currentContainerPath === previousHostPath ||
    (previousDevice !== undefined && currentContainerPath === previousDevice.path);
  if (!untouched) return currentContainerPath;
  return nextDevice ? nextDevice.path : nextHostPath;
}
