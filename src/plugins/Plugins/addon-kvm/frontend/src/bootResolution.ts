// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export interface Resolution {
  width: number;
  height: number;
}

export const BOOT_RESOLUTION_PRESETS: readonly Resolution[] = [
  { width: 1024, height: 768 },
  { width: 1280, height: 720 },
  { width: 1280, height: 800 },
  { width: 1366, height: 768 },
  { width: 1440, height: 900 },
  { width: 1600, height: 900 },
  { width: 1680, height: 1050 },
  { width: 1920, height: 1080 },
  { width: 1920, height: 1200 },
  { width: 2560, height: 1440 },
  { width: 3840, height: 2160 },
];

/** Mirrors validateDisplayResolution in the plugin backend. */
export const BOOT_RESOLUTION_LIMITS = { minWidth: 640, minHeight: 480, maxWidth: 7680, maxHeight: 4320 };

export const DEFAULT_CHOICE = 'default';
export const CUSTOM_CHOICE = 'custom';

export function presetValue({ width, height }: Resolution): string {
  return `${width}x${height}`;
}

/** The dropdown entry a stored size selects: the default, a preset, or Custom. */
export function resolutionChoice(width: number, height: number): string {
  if (width === 0 && height === 0) return DEFAULT_CHOICE;
  const preset = BOOT_RESOLUTION_PRESETS.find((entry) => entry.width === width && entry.height === height);
  return preset ? presetValue(preset) : CUSTOM_CHOICE;
}

/** The size a dropdown entry stands for; null for Custom, which keeps what was typed. */
export function resolutionFromChoice(choice: string): Resolution | null {
  if (choice === DEFAULT_CHOICE) return { width: 0, height: 0 };
  const preset = BOOT_RESOLUTION_PRESETS.find((entry) => presetValue(entry) === choice);
  return preset ? { ...preset } : null;
}

export function isValidResolution(width: number, height: number): boolean {
  if (width === 0 && height === 0) return true;
  const { minWidth, minHeight, maxWidth, maxHeight } = BOOT_RESOLUTION_LIMITS;
  return width >= minWidth && width <= maxWidth && height >= minHeight && height <= maxHeight;
}

/** Only virtio-gpu and QXL take a boot resolution; Auto resolves to one of them. */
export function adapterTakesResolution(displayType: string): boolean {
  return displayType === '' || displayType === 'virtio-gpu' || displayType === 'qxl';
}
