// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useState } from 'react';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useCatalog, useUpdateSchedule } from '../hooks/useReporting';
import { draftComplete, draftCron, draftTargetsPayload, scheduleToDraft, type ScheduleDraft } from './schedule-draft';
import type { SubjectSelection } from './SubjectField';
import type { Level, Schedule } from '../types';

export function useScheduleEdit(schedule: Schedule, onClose: () => void) {
  const catalog = useCatalog();
  const update = useUpdateSchedule();
  const environments = useEnvironmentStore((s) => s.environments);

  const type = catalog.data?.types.find((x) => x.key === schedule.report_type) ?? null;
  const isAdHoc = schedule.report_type === 'ad-hoc';

  const [draft, setDraft] = useState<ScheduleDraft>(() => scheduleToDraft(schedule));
  const [enabled, setEnabled] = useState(schedule.enabled);
  const [envId, setEnvId] = useState(schedule.environment_id);
  const [days, setDays] = useState(String(schedule.period_days || 30));
  const [levels, setLevels] = useState<Level[]>(schedule.levels);
  const [options, setOptions] = useState<string[]>(schedule.options ?? []);
  const [promptText, setPrompt] = useState(schedule.prompt ?? '');

  // Seeded from the stored row: `parseSchedule` on the backend rebuilds the
  // schedule from the request, so a dialog that omitted the scope would
  // silently widen a narrowed report to the whole estate on the first save.
  const [subject, setSubject] = useState<SubjectSelection | null>(
    schedule.subject_kind && (schedule.subject_id || schedule.subject_name)
      ? {
          kind: schedule.subject_kind,
          id: schedule.subject_id ?? '',
          name: schedule.subject_name ?? '',
        }
      : null,
  );
  const onSubjectChange = useCallback((next: SubjectSelection | null) => setSubject(next), []);

  const selectedEnv = environments.find((e) => e.id === envId) ?? null;
  const incomplete = !draftComplete(draft) || (isAdHoc && promptText.trim() === '');

  const submit = () => {
    update.mutate(
      {
        id: schedule.id,
        body: {
          name: draft.name.trim(),
          type: schedule.report_type,
          environment_id: type?.environment_scoped ? envId : '',
          environment: type?.environment_scoped ? (selectedEnv?.name ?? schedule.environment) : '',
          levels: levels.length > 0 ? levels : (type?.levels ?? schedule.levels),
          days: type?.point_in_time ? 0 : Number(days) || 30,
          options,
          subject_kind: subject?.kind ?? '',
          subject_id: subject?.id ?? '',
          subject_name: subject?.name ?? '',
          prompt: isAdHoc ? promptText.trim() : undefined,
          // Carried through rather than re-picked: which AI wrote a
          // scheduled ad-hoc report is what keeps consecutive periods
          // comparable, and an edit to the delivery folder is no reason to
          // change it.
          ai_connection_id: schedule.ai_connection_id,
          format: draft.format,
          cron: draftCron(draft),
          enabled,
          targets: draftTargetsPayload(draft),
        },
      },
      { onSuccess: onClose },
    );
  };

  return {
    update,
    catalog,
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
    incomplete,
    submit,
  };
}

export type ScheduleEditState = ReturnType<typeof useScheduleEdit>;
