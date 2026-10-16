// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Switch } from '@resources/components/ui/Switch';
import { cn } from '@resources/utils/cn';
import type { DeliveryTarget } from '../types';
import {
  FILENAME_TOKENS,
  filenamePatternValid,
  recipientValid,
  storagePathValid,
  type ScheduleTargetDraft,
} from './schedule-draft';

/**
 * One destination, with whatever it lets an operator vary.
 *
 * The override fields belong to the row rather than to the schedule
 * because a schedule can deliver to several destinations, and a folder
 * that suits a NAS share rarely suits an object store — nor does one
 * recipient suit two channels. They appear only once the destination is
 * switched on: an input for something not being written to is noise.
 */
export function TargetRow({
  target,
  selection,
  scheduleName,
  onToggle,
  onFieldChange,
}: {
  target: DeliveryTarget;
  selection: ScheduleTargetDraft | null;
  scheduleName: string;
  onToggle: () => void;
  onFieldChange: (patch: Partial<ScheduleTargetDraft>) => void;
}) {
  const { t } = useTranslation('reporting');
  const id = `reporting-target-${target.kind}-${target.id}`;

  return (
    <div className="rounded-md border border-border/60 bg-card/40 px-3 py-2">
      <div className="flex items-center gap-3">
        <Switch id={id} checked={selection !== null} onCheckedChange={onToggle} aria-label={target.name} />
        <label htmlFor={id} className="min-w-0 flex-1 cursor-pointer text-sm text-foreground">
          <span className="truncate">{target.name}</span>
          <span className="ml-2 text-xs text-muted-foreground">{target.type}</span>
        </label>
        {/* Shown rather than hidden: the operator should see the choice and
            why it is limited, instead of wondering where a channel went. */}
        {!target.supports_files ? (
          <span className="shrink-0 rounded-full bg-muted px-2 py-0.5 text-[10px] font-semibold tracking-wider text-muted-foreground uppercase">
            {t('schedule.linkOnly')}
          </span>
        ) : null}
      </div>

      {selection !== null ? (
        <div className="mt-2 space-y-3 pl-12">
          {target.kind === 'storage' ? (
            <>
              <OverrideField
                id={`${id}-path`}
                label={t('schedule.path')}
                value={selection.path ?? ''}
                maxLength={200}
                placeholder={defaultStorageFolder(scheduleName)}
                valid={storagePathValid(selection.path)}
                hint={t('schedule.pathHint')}
                invalidHint={t('schedule.pathInvalid')}
                onChange={(path) => onFieldChange({ path })}
              />
              <OverrideField
                id={`${id}-filename`}
                label={t('schedule.filename')}
                value={selection.filename ?? ''}
                maxLength={120}
                placeholder={t('schedule.filenamePlaceholder')}
                valid={filenamePatternValid(selection.filename)}
                hint={t('schedule.filenameHint', { tokens: FILENAME_TOKENS.join(' ') })}
                invalidHint={t('schedule.filenameInvalid')}
                onChange={(filename) => onFieldChange({ filename })}
              />
            </>
          ) : null}

          {/* Offered only where the provider genuinely allows it. A Slack
              or Discord webhook has no recipient to vary — its URL is both
              the destination and the secret — so the field is absent
              rather than present and quietly ignored. */}
          {target.kind === 'comm' && target.supports_recipient ? (
            <OverrideField
              id={`${id}-recipient`}
              label={target.recipient_label || t('schedule.recipient')}
              value={selection.recipient ?? ''}
              maxLength={512}
              placeholder={target.recipient_placeholder || t('schedule.recipientPlaceholder')}
              valid={recipientValid(selection.recipient)}
              hint={target.recipient_multiple ? t('schedule.recipientHintMultiple') : t('schedule.recipientHint')}
              invalidHint={t('schedule.recipientInvalid')}
              onChange={(recipient) => onFieldChange({ recipient })}
            />
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

/**
 * One labelled override input, with its own validity state.
 *
 * The three fields differ in copy and bounds and in nothing else, so they
 * share a component: a folder that showed its error differently from a
 * filename would read as two unrelated kinds of mistake.
 */
function OverrideField({
  id,
  label,
  value,
  maxLength,
  placeholder,
  valid,
  hint,
  invalidHint,
  onChange,
}: {
  id: string;
  label: string;
  value: string;
  maxLength: number;
  placeholder: string;
  valid: boolean;
  hint: string;
  invalidHint: string;
  onChange: (value: string) => void;
}) {
  return (
    <div>
      <label htmlFor={id} className="mb-1 block text-xs font-medium text-muted-foreground">
        {label}
      </label>
      <Input
        id={id}
        value={value}
        maxLength={maxLength}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        aria-label={label}
        aria-invalid={!valid}
        className={cn('font-mono text-xs', !valid && 'border-danger')}
      />
      <p className={cn('mt-1 text-xs', valid ? 'text-muted-foreground' : 'text-danger')}>
        {valid ? hint : invalidHint}
      </p>
    </div>
  );
}

/**
 * The folder the backend uses when none is typed, shown as the field's
 * placeholder so "empty" reads as a real default rather than as unset.
 * Mirrors `defaultStoragePath` / `slugify` on the Go side.
 */
function defaultStorageFolder(scheduleName: string): string {
  const slug = scheduleName
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
  return `mcharbor-reports/${slug || 'report'}`;
}
