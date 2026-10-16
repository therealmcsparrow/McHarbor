// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  IconAtom,
  IconBolt,
  IconLeaf,
  IconShieldLock,
  IconSquare,
  IconTerminal,
  IconUser,
  IconUserOff,
  IconUserPlus,
  IconWand,
} from '@tabler/icons-react';

// ShellInfo mirrors the backend's POST /containers/{id}/shells row.
// Kept local to this module so consumers don't have to import the
// hooks barrel just to read the result.
export type ShellInfo = {
  shell: string;
  available: boolean;
};

// Shared shell + user preset lists for the exec/terminal surfaces.
// Labels for root / nobody / shell paths are translation-exempt per
// `.instructions/donottranslate.md` — they are technical identifiers,
// not user-facing prose. The "Container default" / "Other…" labels are
// resolved through `t('exec.userDefault' | 'exec.userOther')` at the
// call site so each language picks the right string.
//
// The "auto-detect" shell label is also rendered verbatim because it
// only ever appears in the dropdown alongside the shell paths and the
// behavior is identical across locales (we still call detectShell on
// the backend) — leaving it as English avoids translating a value that
// the backend will silently overwrite anyway.

export type TerminalShellOption = {
  value: string;
  label: string;
  icon: typeof IconTerminal;
  iconClassName: string;
};

export const SHELL_OPTIONS: TerminalShellOption[] = [
  { value: '', label: 'Auto-detect', icon: IconWand, iconClassName: 'text-primary' },
  { value: '/bin/sh', label: '/bin/sh', icon: IconTerminal, iconClassName: 'text-muted-foreground' },
  { value: '/bin/bash', label: '/bin/bash', icon: IconBolt, iconClassName: 'text-success' },
  { value: '/bin/ash', label: '/bin/ash', icon: IconLeaf, iconClassName: 'text-success' },
  { value: '/bin/dash', label: '/bin/dash', icon: IconSquare, iconClassName: 'text-chart-2' },
  { value: '/bin/zsh', label: '/bin/zsh', icon: IconAtom, iconClassName: 'text-info' },
];

// Sentinel values used by USER_OPTIONS — the resolver in each
// component maps these to the actual exec instance User field
// (empty string = let the image's USER apply).
export const USER_PRESET_DEFAULT = '__default__';
export const USER_PRESET_OTHER = '__other__';

export type TerminalUserOption = {
  value: string;
  // Label is resolved at render time via the i18n key passed by the
  // caller. Components that already have a `t` in scope can build the
  // list inline; this helper just centralizes the icons + colors so
  // shell + user dropdowns look identical across surfaces.
  label: string;
  icon: typeof IconUser;
  iconClassName: string;
};

export const USER_ICON = {
  default: IconUser,
  root: IconShieldLock,
  nobody: IconUserOff,
  other: IconUserPlus,
} as const;

export const USER_ICON_CLASS = {
  default: 'text-muted-foreground',
  root: 'text-danger',
  nobody: 'text-muted-foreground',
  other: 'text-info',
} as const;

// filterApplicableShells returns the subset of SHELL_OPTIONS that the
// container actually has installed. The "Auto-detect" entry (empty
// value) is always preserved — the backend always falls back to
// /bin/sh or whatever it finds, so it cannot be wrong.
//
// Pass an empty list (the "we haven't probed yet" case) to keep every
// option visible — better to over-show than to silently hide the only
// shell the container has.
export function filterApplicableShells(detected: ShellInfo[] | undefined | null): TerminalShellOption[] {
  if (!detected || detected.length === 0) {
    return SHELL_OPTIONS;
  }
  const available = new Set(detected.filter((s) => s.available).map((s) => s.shell));
  return SHELL_OPTIONS.filter((opt) => opt.value === '' || available.has(opt.value));
}

// USER_OPTIONS is the shared list of user-preset dropdown entries.
// The icon + color for each preset is centralized in USER_ICON /
// USER_ICON_CLASS so every surface renders consistently. Labels are
// resolved at render time via the i18n key passed by the caller
// (TerminalPage / TerminalTab each have their own t() in scope).
export const USER_OPTIONS: TerminalUserOption[] = [
  {
    value: USER_PRESET_DEFAULT,
    label: 'Container default',
    icon: USER_ICON.default,
    iconClassName: USER_ICON_CLASS.default,
  },
  { value: 'root', label: 'root', icon: USER_ICON.root, iconClassName: USER_ICON_CLASS.root },
  { value: 'nobody', label: 'nobody', icon: USER_ICON.nobody, iconClassName: USER_ICON_CLASS.nobody },
  {
    value: USER_PRESET_OTHER,
    label: 'Custom…',
    icon: USER_ICON.other,
    iconClassName: USER_ICON_CLASS.other,
  },
];

// buildUserOptions is the translated form of USER_OPTIONS, for surfaces whose
// "default" preset needs its own wording - a host terminal starts as root, not
// as an image's USER, so "Container default" would be a lie there.
//
// The two middle presets stay verbatim: `root` and `nobody` are account names,
// not prose, and are translation-exempt per `.instructions/donottranslate.md`.
export function buildUserOptions(defaultLabel: string, customLabel: string): TerminalUserOption[] {
  return [
    {
      value: USER_PRESET_DEFAULT,
      label: defaultLabel,
      icon: USER_ICON.default,
      iconClassName: USER_ICON_CLASS.default,
    },
    { value: 'root', label: 'root', icon: USER_ICON.root, iconClassName: USER_ICON_CLASS.root },
    { value: 'nobody', label: 'nobody', icon: USER_ICON.nobody, iconClassName: USER_ICON_CLASS.nobody },
    {
      value: USER_PRESET_OTHER,
      label: customLabel,
      icon: USER_ICON.other,
      iconClassName: USER_ICON_CLASS.other,
    },
  ];
}
