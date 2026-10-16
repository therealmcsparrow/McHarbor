// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Select } from '@resources/components/ui/Select';
import { Input } from '@resources/components/ui/Input';
import { useDeliveryTargets } from '../hooks/useReporting';
import type { DeliveryTarget } from '../types';
import { Field } from './ScheduleField';
import { FrequencyFields } from './ScheduleFrequency';
import { TargetRow } from './ScheduleTargets';
import type { ScheduleDraft, ScheduleTargetDraft } from './schedule-draft';

/**
 * Name, format, cadence and destinations.
 *
 * Shared by the studio's opt-in block and the schedules page's edit
 * dialog, so the two can never disagree about what a schedule is.
 */
export function ScheduleFields({ draft, onChange }: { draft: ScheduleDraft; onChange: (next: ScheduleDraft) => void }) {
  const { t } = useTranslation('reporting');
  // Enabled unconditionally: this component only renders once the operator
  // is actually configuring a schedule, so the lookup is never speculative.
  const targets = useDeliveryTargets(true);

  const options = useMemo<DeliveryTarget[]>(
    () => [...(targets.data?.channels ?? []), ...(targets.data?.locations ?? [])],
    [targets.data],
  );

  // A target that disappears between the list loading and the schedule
  // being saved would be rejected by the backend, so prune the selection
  // against what actually exists rather than sending a stale id.
  useEffect(() => {
    if (targets.data === undefined) return;
    const known = new Map(options.map((o) => [`${o.kind}:${o.id}`, o]));
    const kept = draft.targets.filter((sel) => known.has(`${sel.kind}:${sel.ref_id}`));
    // A recipient override on a channel that has since lost the
    // capability - the adapter was rebuilt, or the channel type changed
    // hands - is cleared for the same reason a vanished destination is
    // dropped: the backend refuses such an override, so leaving it in the
    // draft would make an otherwise valid schedule impossible to save,
    // and the operator would have no field to clear it from.
    const cleaned = kept.map((sel) => {
      const option = known.get(`${sel.kind}:${sel.ref_id}`);
      if (sel.kind !== 'comm' || option?.supports_recipient || !sel.recipient) return sel;
      return { ...sel, recipient: '' };
    });
    const changed = cleaned.length !== draft.targets.length || cleaned.some((sel, i) => sel !== draft.targets[i]);
    if (changed) onChange({ ...draft, targets: cleaned });
  }, [draft, onChange, options, targets.data]);

  const toggleTarget = (target: DeliveryTarget) => {
    const selected = draft.targets.some((sel) => sel.kind === target.kind && sel.ref_id === target.id);
    onChange({
      ...draft,
      targets: selected
        ? draft.targets.filter((sel) => !(sel.kind === target.kind && sel.ref_id === target.id))
        : [...draft.targets, { kind: target.kind, ref_id: target.id, path: '', filename: '', recipient: '' }],
    });
  };

  // One setter for all three per-target fields: they differ only in which
  // key they write, and three near-identical closures would drift.
  const setTargetField = (
    target: DeliveryTarget,
    patch: Partial<Pick<ScheduleTargetDraft, 'path' | 'filename' | 'recipient'>>,
  ) => {
    onChange({
      ...draft,
      targets: draft.targets.map((sel) =>
        sel.kind === target.kind && sel.ref_id === target.id ? { ...sel, ...patch } : sel,
      ),
    });
  };

  const noDestinations = targets.data !== undefined && options.length === 0;

  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t('schedule.name')}>
          <Input
            value={draft.name}
            maxLength={80}
            onChange={(e) => onChange({ ...draft, name: e.target.value })}
            aria-label={t('schedule.name')}
          />
        </Field>
        <Field label={t('schedule.format')}>
          <Select
            value={draft.format}
            onChange={(value) => onChange({ ...draft, format: value as 'pdf' | 'docx' })}
            ariaLabel={t('schedule.format')}
            options={[
              { value: 'pdf', label: t('schedule.formatPdf') },
              { value: 'docx', label: t('schedule.formatDocx') },
            ]}
          />
        </Field>
      </div>

      <FrequencyFields draft={draft} onChange={onChange} />

      <Field label={t('schedule.targets')}>
        {targets.isLoading ? (
          <p className="text-sm text-muted-foreground">{t('schedule.targetsLoading')}</p>
        ) : noDestinations ? (
          <p className="text-sm text-muted-foreground">{t('schedule.targetsEmpty')}</p>
        ) : (
          <div className="space-y-2">
            {options.map((target) => (
              <TargetRow
                key={`${target.kind}:${target.id}`}
                target={target}
                selection={draft.targets.find((sel) => sel.kind === target.kind && sel.ref_id === target.id) ?? null}
                scheduleName={draft.name}
                onToggle={() => toggleTarget(target)}
                onFieldChange={(patch) => setTargetField(target, patch)}
              />
            ))}
          </div>
        )}
        <p className="mt-2 text-xs text-muted-foreground">{t('schedule.targetsHint')}</p>
      </Field>

      <p className="flex items-start gap-2 rounded-md bg-warning/10 p-3 text-xs text-foreground">
        <IconAlertTriangle className="mt-0.5 size-4 shrink-0 text-warning" aria-hidden="true" />
        {t('schedule.libraryNotice')}
      </p>
    </div>
  );
}
