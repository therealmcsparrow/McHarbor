// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { cn } from '@resources/utils/cn';
import { SubjectField } from './SubjectField';
import { typeName } from './catalog-labels';
import { ScheduleSection } from './ScheduleSection';
import { Field } from './ScheduleField';
import { ToggleField } from './ToggleField';
import { optionLabel } from './option-label';
import type { ConfigureState } from './useConfigureState';
import type { ReportType } from '../types';

export function ConfigureFields({ type, state }: { type: ReportType; state: ConfigureState }) {
  const { t, i18n } = useTranslation('reporting');
  const {
    isAdHoc,
    promptText,
    setPrompt,
    envId,
    setEnvId,
    environments,
    period,
    setPeriod,
    from,
    setFrom,
    to,
    setTo,
    levels,
    options,
    onSubjectChange,
    canSchedule,
    schedule,
    setSchedule,
    needsAI,
    aiConnections,
    aiSettled,
    aiConnectionList,
    aiConnectionId,
    setAiConnectionId,
    custom,
    periodOptions,
    toggleLevel,
    toggleOption,
  } = state;

  return (
    <>
      {isAdHoc ? (
        <Field label={t('studio.prompt')}>
          <textarea
            value={promptText}
            onChange={(e) => setPrompt(e.target.value)}
            rows={4}
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
        {!isAdHoc ? (
          <Field label={t('studio.period')}>
            {type.point_in_time ? (
              <p className="text-sm text-foreground">{t('studio.pointInTimeHint')}</p>
            ) : (
              <Select value={period} onChange={setPeriod} ariaLabel={t('studio.period')} options={periodOptions} />
            )}
          </Field>
        ) : null}

        {type.environment_scoped ? (
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

        {/* The scope sits with period and environment because it is the
              same kind of question — which slice of the estate — and a
              reader answering all three answers them in one place. For
              ad-hoc the model derives the scope from the question, so
              this control is not offered. */}
        {!isAdHoc && (type.subjects ?? []).length > 0 ? (
          <SubjectField
            kinds={type.subjects ?? []}
            envId={type.environment_scoped ? envId : ''}
            labelNs={type.label_ns || type.provider}
            onChange={onSubjectChange}
          />
        ) : null}
      </div>

      {!isAdHoc && custom && !type.point_in_time ? (
        <div className="grid gap-5 sm:grid-cols-2">
          <Field label={t('studio.from')}>
            <Input
              type="date"
              value={from}
              max={to || undefined}
              onChange={(e) => setFrom(e.target.value)}
              aria-label={t('studio.from')}
            />
          </Field>
          <Field label={t('studio.to')}>
            <Input
              type="date"
              value={to}
              min={from || undefined}
              onChange={(e) => setTo(e.target.value)}
              aria-label={t('studio.to')}
            />
          </Field>
        </div>
      ) : null}

      <Field label={t('studio.sections')}>
        <div className="flex flex-wrap gap-x-6 gap-y-3">
          {type.levels.map((level) => (
            <ToggleField
              key={level}
              id={`reporting-level-${level}`}
              label={t(`level.${level}`)}
              checked={levels.includes(level)}
              onToggle={() => toggleLevel(level)}
            />
          ))}
        </div>
      </Field>

      <Field label={t('studio.options')}>
        <div className="flex flex-wrap gap-x-6 gap-y-3">
          {(type.options ?? []).map((option) => (
            <ToggleField
              key={option.key}
              id={`reporting-option-${option.key}`}
              label={optionLabel(t, (k, o) => i18n.exists(k, o), type, option.key)}
              checked={options.includes(option.key)}
              onToggle={() => toggleOption(option.key)}
            />
          ))}
        </div>
      </Field>

      {canSchedule ? <ScheduleSection draft={schedule} onChange={setSchedule} defaultName={typeName(t, type)} /> : null}

      {/* Hidden while the connection list is loading or failed to load
            — a slow or broken lookup should never flash an empty control
            or block the rest of the dialog. */}
      {needsAI && aiSettled ? (
        <Field label={t('studio.aiConnection')}>
          {aiConnections.data?.available && aiConnectionList.length > 0 ? (
            <Select
              value={aiConnectionId}
              onChange={setAiConnectionId}
              ariaLabel={t('studio.aiConnection')}
              options={aiConnectionList.map((c) => ({
                value: c.id,
                label: c.provider_type && c.provider_type !== c.name ? `${c.name} (${c.provider_type})` : c.name,
              }))}
            />
          ) : (
            <p className="text-sm text-muted-foreground">
              {isAdHoc ? t('studio.aiConnectionRequired') : t('studio.aiConnectionHint')}
            </p>
          )}
        </Field>
      ) : null}
    </>
  );
}
