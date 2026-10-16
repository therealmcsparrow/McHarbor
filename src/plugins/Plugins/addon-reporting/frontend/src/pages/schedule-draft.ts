// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Schedule, ScheduleTargetKind } from '../types';

export type ScheduleTargetDraft = {
  kind: ScheduleTargetKind;
  ref_id: string;
  /** Storage targets only. Empty means the backend's default folder. */
  path?: string;
  /** Storage targets only. Empty means the exporter's own dated name. */
  filename?: string;
  /** Comm targets only. Empty means the channel's own destination. */
  recipient?: string;
};

export type ScheduleDraft = {
  name: string;
  frequency: Frequency;
  hour: number;
  weekday: number;
  dayOfMonth: number;
  customCron: string;
  format: 'pdf' | 'docx';
  targets: ScheduleTargetDraft[];
};

export type Frequency = 'daily' | 'weekly' | 'monthly' | 'custom';

export function emptyScheduleDraft(itemName: string): ScheduleDraft {
  return {
    name: itemName,
    frequency: 'weekly',
    hour: 7,
    weekday: 1,
    dayOfMonth: 1,
    customCron: '0 7 * * 1',
    format: 'pdf',
    targets: [],
  };
}

/**
 * A stored schedule, back as a draft.
 *
 * The cron is parsed into the preset controls where it fits one, so
 * editing "every Monday at 07:00" shows exactly that rather than dropping
 * every schedule into the raw-expression box. Anything the presets cannot
 * express — a step, a list, several hours — stays `custom` with the
 * expression untouched: rewriting an operator's expression into the
 * nearest preset would change when their report runs.
 */
export function scheduleToDraft(schedule: Schedule): ScheduleDraft {
  const base: ScheduleDraft = {
    ...emptyScheduleDraft(schedule.name),
    format: schedule.format,
    customCron: schedule.cron,
    frequency: 'custom',
    targets: schedule.targets.map((target) => ({
      kind: target.kind,
      ref_id: target.ref_id,
      path: target.path ?? '',
      filename: target.filename ?? '',
      recipient: target.recipient ?? '',
    })),
  };

  const [minute, hour, dayOfMonth, month, weekday] = schedule.cron.trim().split(/\s+/);
  // Only the exact shapes `draftCron` produces are recognised. A partial
  // match would be worse than none: it would show a preset meaning
  // something slightly different from what is actually stored.
  if (minute !== '0' || month !== '*' || !/^\d{1,2}$/.test(hour ?? '')) return base;

  const at = { ...base, hour: Number(hour) };
  if (dayOfMonth === '*' && weekday === '*') {
    return { ...at, frequency: 'daily' };
  }
  if (dayOfMonth === '*' && /^[0-6]$/.test(weekday ?? '')) {
    return { ...at, frequency: 'weekly', weekday: Number(weekday) };
  }
  const day = Number(dayOfMonth);
  if (weekday === '*' && /^\d{1,2}$/.test(dayOfMonth ?? '') && day >= 1 && day <= 28) {
    return { ...at, frequency: 'monthly', dayOfMonth: day };
  }
  return base;
}

/**
 * The targets half of a create or update request.
 *
 * A blank folder is sent as absent rather than as an empty string: the
 * backend reads "no path" as "use the default", and writing the default
 * into the row would freeze it against a later rename of the schedule.
 */
export function draftTargetsPayload(draft: ScheduleDraft) {
  return draft.targets.map((target) => ({
    kind: target.kind,
    ref_id: target.ref_id,
    path: target.path?.trim() ? target.path.trim() : undefined,
    filename: target.filename?.trim() ? target.filename.trim() : undefined,
    recipient: target.recipient?.trim() ? target.recipient.trim() : undefined,
  }));
}

/**
 * The cron a draft means, in UTC — the same timezone the backend evaluates
 * in. Shown to the operator rather than hidden, so a schedule that fires
 * at an unexpected hour can be diagnosed from the form that created it.
 */
export function draftCron(draft: ScheduleDraft): string {
  const hour = clampHour(draft.hour);
  switch (draft.frequency) {
    case 'daily':
      return `0 ${hour} * * *`;
    case 'weekly':
      return `0 ${hour} * * ${clamp(draft.weekday, 0, 6)}`;
    case 'monthly':
      return `0 ${hour} ${clamp(draft.dayOfMonth, 1, 28)} * *`;
    case 'custom':
      return draft.customCron.trim();
  }
}

/** A five-field expression, checked the way the backend checks it. */
export function draftCronValid(draft: ScheduleDraft): boolean {
  const cron = draftCron(draft);
  return cron.split(/\s+/).filter(Boolean).length === 5;
}

/**
 * Whether a typed storage folder is one the backend will accept.
 *
 * Deliberately the same rules as `sanitiseStoragePath` in the Go side, and
 * deliberately not a replacement for it: this exists so the operator sees
 * the problem next to the field they typed it in, rather than as a toast
 * after pressing the button. The server still decides.
 */
export function storagePathValid(path: string | undefined): boolean {
  const p = (path ?? '').trim();
  if (p === '') return true; // empty means "use the default folder"
  if (p.length > 200) return false;
  // A leading slash or a drive letter both say "ignore the location's base
  // path", which is not a decision a schedule gets to make.
  if (p.startsWith('/') || p.startsWith('\\') || p.includes(':')) return false;
  if (/[\x00-\x1F\x7F]/.test(p)) return false;
  return p
    .replace(/\\/g, '/')
    .split('/')
    .every((segment) => segment.trim() !== '..');
}

/** Every storage folder in the draft is acceptable. */
export function draftPathsValid(draft: ScheduleDraft): boolean {
  return draft.targets.every((target) => storagePathValid(target.path));
}

/** The tokens `applyFilenamePattern` substitutes on the Go side. */
export const FILENAME_TOKENS = ['{name}', '{type}', '{env}', '{date}', '{time}'] as const;

/**
 * Whether a typed filename pattern is one the backend will accept.
 *
 * Mirrors `sanitiseFilenamePattern` in Go, and exists for the same reason
 * `storagePathValid` does: so the operator sees the problem beside the
 * field rather than as a toast. A separator is refused rather than
 * stripped — the folder is the path field, and a name carrying a slash
 * would write outside it — and so is an extension, because the plugin
 * appends the one matching the bytes it actually rendered.
 */
export function filenamePatternValid(pattern: string | undefined): boolean {
  const p = (pattern ?? '').trim();
  if (p === '') return true; // empty means "the exporter's own name"
  if (p.length > 120) return false;
  if (/[\x00-\x1F\x7F]/.test(p)) return false;
  if (/[/\\:]/.test(p)) return false;
  if (p.includes('..') || p.startsWith('.')) return false;
  if (/\.(pdf|docx)$/i.test(p)) return false;
  // Any brace that is not a known token is a typo, and a folder full of
  // `report-{dat}.pdf` is a mistake nobody notices until they go looking.
  const braces = p.match(/\{[^{}]*\}/g) ?? [];
  if (!braces.every((token) => (FILENAME_TOKENS as readonly string[]).includes(token))) return false;
  return !/[{}]/.test(p.replace(/\{[^{}]*\}/g, ''));
}

/**
 * Whether a typed alternative recipient is one the backend will accept.
 *
 * Only length and control characters are checked. A newline is the whole
 * risk — several adapters write this value into a header, an SMTP `To:`
 * above all — while the format itself belongs to the provider: guessing
 * one here would refuse valid destinations.
 */
export function recipientValid(recipient: string | undefined): boolean {
  const v = (recipient ?? '').trim();
  if (v === '') return true; // empty means "the channel's own destination"
  if (v.length > 512) return false;
  return !/[\x00-\x1F\x7F]/.test(v);
}

/** Every per-target override in the draft is acceptable. */
export function draftOverridesValid(draft: ScheduleDraft): boolean {
  return draft.targets.every((target) => filenamePatternValid(target.filename) && recipientValid(target.recipient));
}

/** Name, cadence and at least one destination — the save gate both dialogs use. */
export function draftComplete(draft: ScheduleDraft): boolean {
  return (
    draft.name.trim() !== '' &&
    draft.targets.length > 0 &&
    draftCronValid(draft) &&
    draftPathsValid(draft) &&
    draftOverridesValid(draft)
  );
}

export function clamp(value: number, lo: number, hi: number): number {
  if (!Number.isFinite(value)) return lo;
  return Math.min(Math.max(Math.trunc(value), lo), hi);
}

export function clampHour(value: number): number {
  return clamp(value, 0, 23);
}
