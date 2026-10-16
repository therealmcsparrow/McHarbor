// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useEnvironmentStore } from '@resources/stores/environment';
import { usePermission } from '@core/auth/usePermission';
import { useAIConnections } from '../hooks/useReporting';
import type { SubjectSelection } from './SubjectField';
import { CUSTOM, PERIODS, isoDaysAgo, periodDays, periodFields } from './configure-helpers';
import { draftCron, draftTargetsPayload, draftComplete, type ScheduleDraft } from './schedule-draft';
import type { GenerateRequest, Level, ReportType, ScheduleRequest } from '../types';

type Args = {
  type: ReportType;
  onGenerate: (request: GenerateRequest) => void;
  onSchedule: (request: ScheduleRequest) => void;
};

export function useConfigureState({ type, onGenerate, onSchedule }: Args) {
  const { t } = useTranslation('reporting');
  const currentId = useEnvironmentStore((s) => s.currentId);
  const environments = useEnvironmentStore((s) => s.environments);

  // For ad-hoc the model plans the period and the subject from the
  // question itself, so this type asks for a prompt instead of those
  // controls. It still asks for environment and options — the model
  // does not get to pick what the user is allowed to see.
  const isAdHoc = type.key === 'ad-hoc';
  const [promptText, setPrompt] = useState('');

  const [envId, setEnvId] = useState<string>(
    currentId || environments.find((e) => e.isDefault)?.id || environments[0]?.id || '',
  );
  const [period, setPeriod] = useState<string>(String(type.default_days > 0 ? type.default_days : 30));
  const [from, setFrom] = useState<string>(isoDaysAgo(type.default_days > 0 ? type.default_days : 30));
  const [to, setTo] = useState<string>(isoDaysAgo(0));
  const [levels, setLevels] = useState<Level[]>(type.levels);
  const [options, setOptions] = useState<string[]>((type.options ?? []).filter((o) => o.default).map((o) => o.key));
  const [subject, setSubject] = useState<SubjectSelection | null>(null);

  // Scheduling is a separate permission from generating, so a user who may
  // run a report on demand does not automatically get to automate one.
  // The section is absent rather than disabled here: an entire block of
  // controls rendered dead is noise, and the sidebar entry that leads to
  // schedules is gated on the same key.
  const canSchedule = usePermission('reporting.schedule');
  const [schedule, setSchedule] = useState<ScheduleDraft | null>(null);

  // Stable, because `SubjectField` clears the selection from an effect
  // keyed on the kind and the environment; a new identity every render
  // would make that effect fire on every render.
  const onSubjectChange = useCallback((next: SubjectSelection | null) => setSubject(next), []);

  // Generation uses AI in exactly two cases: the `ad-hoc` type always
  // (a model plans and writes the whole document), and any other type
  // when narrative polish is checked (it calls AI to reword the opening
  // paragraph). Everything else renders deterministically and never
  // needs a connection.
  const needsAI = isAdHoc || options.includes('include_narrative');
  const aiConnections = useAIConnections();
  const [pickedAiConnectionId, setAiConnectionId] = useState('');

  // Default to the operator's favourite connection once the list loads,
  // without ever overwriting a choice the operator already made — this
  // only fills the empty state.
  const connectionChoices = aiConnections.data?.connections ?? [];
  const favouriteConnection = connectionChoices.find((c) => c.is_default) ?? connectionChoices[0];
  const aiConnectionId = pickedAiConnectionId || favouriteConnection?.id || '';

  // Settled means the query neither is loading nor has failed — only
  // then do we know for certain whether AI is actually usable. Until
  // then the selector stays hidden and generation stays unblocked, so a
  // slow or failed lookup never freezes or breaks the dialog.
  const aiSettled = !aiConnections.isLoading && !aiConnections.isError;
  const aiConnectionList = aiConnections.data?.connections ?? [];
  const aiUnavailable = aiSettled && needsAI && (!aiConnections.data?.available || aiConnectionList.length === 0);

  const selectedEnv = environments.find((e) => e.id === envId) ?? null;
  const custom = period === CUSTOM;
  // A same-day range is legitimate — the backend takes `to` as the end of
  // that day — so only an inverted range is rejected.
  const rangeInvalid = custom && (!from || !to || from > to);

  const periodOptions = useMemo(
    () => [
      ...PERIODS.map((d) => ({ value: String(d), label: t('studio.lastDays', { count: d }) })),
      { value: CUSTOM, label: t('studio.customRange') },
    ],
    [t],
  );

  const toggleLevel = (level: Level) =>
    setLevels((prev) => (prev.includes(level) ? prev.filter((l) => l !== level) : [...prev, level]));

  const toggleOption = (key: string) =>
    setOptions((prev) => (prev.includes(key) ? prev.filter((o) => o !== key) : [...prev, key]));

  // The same gate the edit dialog applies, so a schedule cannot be created
  // in a state the edit dialog would refuse to save.
  const scheduleInvalid = schedule !== null && !draftComplete(schedule);

  const submit = () => {
    if (schedule) {
      onSchedule({
        name: schedule.name.trim(),
        type: type.key,
        environment_id: type.environment_scoped ? envId : '',
        environment: type.environment_scoped ? (selectedEnv?.name ?? '') : '',
        levels: levels.length > 0 ? levels : type.levels,
        // A schedule stores a *window length*, not a fixed range: every
        // run must cover the period ending at its own run time, or the
        // second month would report the first month again.
        days: type.point_in_time ? 0 : periodDays(period, from, to),
        options,
        subject_kind: subject?.kind ?? '',
        subject_id: subject?.id ?? '',
        subject_name: subject?.name ?? '',
        prompt: isAdHoc ? promptText.trim() : undefined,
        ai_connection_id: needsAI ? aiConnectionId : undefined,
        format: schedule.format,
        cron: draftCron(schedule),
        enabled: true,
        targets: draftTargetsPayload(schedule),
      });
      return;
    }
    onGenerate({
      type: type.key,
      environment_id: type.environment_scoped ? envId : '',
      environment: type.environment_scoped ? (selectedEnv?.name ?? '') : '',
      // An empty level list would mean "everything"; the backend treats
      // it that way too, so unchecking all of them is not a dead end.
      levels: levels.length > 0 ? levels : type.levels,
      // The period control is hidden for ad-hoc (the model plans the
      // window from the question itself), so nothing here reflects user
      // intent for it — sending it anyway would silently override the
      // plan's own Days on the backend.
      ...(isAdHoc ? {} : periodFields(type, period, from, to)),
      options,
      subject_kind: subject?.kind ?? '',
      subject_id: subject?.id ?? '',
      subject_name: subject?.name ?? '',
      prompt: isAdHoc ? promptText.trim() : undefined,
      // Empty/omitted means "use the host's favourite connection" — sent
      // only when this generation will actually call AI.
      ai_connection_id: needsAI ? aiConnectionId : undefined,
    });
  };

  return {
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
    subject,
    onSubjectChange,
    canSchedule,
    schedule,
    setSchedule,
    needsAI,
    aiConnections,
    aiSettled,
    aiConnectionList,
    aiUnavailable,
    aiConnectionId,
    setAiConnectionId,
    selectedEnv,
    custom,
    rangeInvalid,
    periodOptions,
    toggleLevel,
    toggleOption,
    scheduleInvalid,
    submit,
  };
}

export type ConfigureState = ReturnType<typeof useConfigureState>;
