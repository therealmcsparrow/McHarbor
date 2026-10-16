// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { cn } from '@resources/utils/cn';
import { Field } from './ScheduleField';
import { ScheduleFields } from './ScheduleFields';
import { SubjectField } from './SubjectField';
import { ToggleField } from './ToggleField';
import { optionLabel } from './option-label';
import type { ScheduleEditState } from './useScheduleEdit';

export function ScheduleEditFields({ state }: { state: ScheduleEditState }) {
  const { t, i18n } = useTranslation('reporting');
  const {
    type,
    isAdHoc,
    environments,
    draft,
    setDraft,
    enabled,
    setEnabled,
    envId,
    setEnvId,
    days,
    setDays,
    levels,
    setLevels,
    options,
    setOptions,
    promptText,
    setPrompt,
    subject,
    onSubjectChange,
  } = state;

  return (
    <>
      <div className="flex items-center gap-3">
        <Switch
          id="reporting-schedule-enabled"
          checked={enabled}
          onCheckedChange={setEnabled}
          aria-label={t('schedules.edit.enabled')}
        />
        <label htmlFor="reporting-schedule-enabled" className="cursor-pointer text-sm text-foreground">
          {t('schedules.edit.enabled')}
        </label>
      </div>

      {isAdHoc ? (
        <Field label={t('studio.prompt')}>
          <textarea
            value={promptText}
            onChange={(e) => setPrompt(e.target.value)}
            rows={3}
            maxLength={2000}
            placeholder={t('studio.promptPlaceholder')}
            aria-label={t('studio.prompt')}
            className={cn(
              'w-full rounded-md border border-border bg-card px-3 py-2',
              'text-sm text-foreground placeholder:text-muted-foreground',
              'focus:ring-2 focus:ring-ring focus:outline-none',
            )}
          />
        </Field>
      ) : null}

      <div className="grid gap-5 sm:grid-cols-2">
        {/* Ad-hoc plans its own window from the question, and a
              point-in-time report has none, so neither is asked. */}
        {!isAdHoc && !type?.point_in_time ? (
          <Field label={t('schedules.edit.period')}>
            <NumberInput
              nullable
              min={1}
              max={366}
              value={days.trim() === '' ? null : Number(days)}
              onChange={(next) => setDays(next === null ? '' : String(next))}
              aria-label={t('schedules.edit.period')}
            />
            <p className="mt-1.5 text-xs text-muted-foreground">{t('schedules.edit.periodHint')}</p>
          </Field>
        ) : null}

        {type?.environment_scoped ? (
          <Field label={t('studio.environment')}>
            <Select
              value={envId}
              onChange={setEnvId}
              ariaLabel={t('studio.environment')}
              options={[
                { value: '', label: t('studio.environmentAll') },
                ...environments.map((e) => ({ value: e.id, label: e.name })),
              ]}
            />
          </Field>
        ) : null}

        {!isAdHoc && (type?.subjects ?? []).length > 0 ? (
          <SubjectField
            kinds={type?.subjects ?? []}
            envId={type?.environment_scoped ? envId : ''}
            initial={subject}
            onChange={onSubjectChange}
          />
        ) : null}
      </div>

      {type ? (
        <>
          <Field label={t('studio.sections')}>
            <div className="flex flex-wrap gap-x-6 gap-y-3">
              {type.levels.map((level) => (
                <ToggleField
                  key={level}
                  id={`reporting-edit-level-${level}`}
                  label={t(`level.${level}`)}
                  checked={levels.includes(level)}
                  onToggle={() =>
                    setLevels((prev) => (prev.includes(level) ? prev.filter((l) => l !== level) : [...prev, level]))
                  }
                />
              ))}
            </div>
          </Field>

          <Field label={t('studio.options')}>
            <div className="flex flex-wrap gap-x-6 gap-y-3">
              {(type.options ?? []).map((option) => (
                <ToggleField
                  key={option.key}
                  id={`reporting-edit-option-${option.key}`}
                  label={optionLabel(t, (k, o) => i18n.exists(k, o), type, option.key)}
                  checked={options.includes(option.key)}
                  onToggle={() =>
                    setOptions((prev) =>
                      prev.includes(option.key) ? prev.filter((o) => o !== option.key) : [...prev, option.key],
                    )
                  }
                />
              ))}
            </div>
          </Field>
        </>
      ) : null}

      <ScheduleFields draft={draft} onChange={setDraft} />
    </>
  );
}
