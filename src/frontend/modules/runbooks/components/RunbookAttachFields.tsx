// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Select } from '@resources/components/ui/Select';
import { useAlertConditions } from '@resources/hooks/useAlertConditions';
import { useAttachLabel } from '../hooks/useAttachLabel';
import { RunbookPickField } from './RunbookPickField';
import { RunbookTargetField } from './RunbookTargetField';
import {
  HOST_ATTACH_KINDS,
  type AttachKind,
  type RunbookCatalog,
  type RunbookInput,
  type TargetKind,
} from '@resources/types/runbooks';

type RunbookAttachFieldsProps = {
  draft: RunbookInput;
  catalog: RunbookCatalog | undefined;
  envId: string;
  patch: (next: Partial<RunbookInput>) => void;
};

function subjectKind(kind: AttachKind, catalog: RunbookCatalog | undefined): TargetKind | undefined {
  if (kind === 'container' || kind === 'stack') return kind;
  return catalog?.attachKinds?.find((entry) => entry.kind === kind)?.selector;
}

export function RunbookAttachFields({ draft, catalog, envId, patch }: RunbookAttachFieldsProps) {
  const { t } = useTranslation('runbooks');
  const conditions = useAlertConditions();
  const attachLabel = useAttachLabel();
  const orphan = catalog !== undefined && attachLabel(draft.attachKind).orphan;

  const kinds = useMemo(() => {
    const declared = (catalog?.attachKinds ?? []).map((entry) => entry.kind);
    const stored = [...HOST_ATTACH_KINDS, ...declared] as AttachKind[];
    const all = stored.includes(draft.attachKind) ? stored : [...stored, draft.attachKind];
    return all.map((kind) => ({ value: kind, label: attachLabel(kind).label }));
  }, [attachLabel, catalog?.attachKinds, draft.attachKind]);

  const sources = conditions.data?.sources ?? [];
  const sourceOptions = sources.map((source) => ({
    value: source.sourceType,
    label: source.displayName,
    description: source.description,
  }));
  const conditionOptions = (
    sources.find((source) => source.sourceType === draft.attachSourceType)?.conditions ?? []
  ).map((condition) => ({ value: condition.key, label: condition.displayName, description: condition.description }));
  const subject = subjectKind(draft.attachKind, catalog);

  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <Select
        ariaLabel={t('editor.attachKind')}
        value={draft.attachKind}
        onChange={(value) => patch({ attachKind: value as AttachKind, attachSubject: '' })}
        options={kinds}
      />
      {orphan && (
        <p role="alert" className="text-xs text-destructive sm:col-span-2">
          {t('editor.attachOrphan', { kind: draft.attachKind })}
        </p>
      )}
      {draft.attachKind === 'alert_condition' && (
        <>
          <RunbookPickField
            key={`source-${conditions.isSuccess}`}
            ariaLabel={t('editor.sourceType')}
            placeholder={t('editor.pickSource')}
            value={draft.attachSourceType}
            options={sourceOptions}
            onChange={(value) => patch({ attachSourceType: value, attachConditionKey: '' })}
          />
          <RunbookPickField
            key={`condition-${conditions.isSuccess}-${draft.attachSourceType}`}
            ariaLabel={t('editor.conditionKey')}
            placeholder={t('editor.pickCondition')}
            value={draft.attachConditionKey}
            options={conditionOptions}
            onChange={(value) => patch({ attachConditionKey: value })}
          />
        </>
      )}
      {subject && (
        <RunbookTargetField
          key={`${draft.attachKind}-${envId}`}
          kind={subject}
          value={draft.attachSubject}
          onChange={(value) => patch({ attachSubject: value })}
          envId={envId}
          ariaLabel={t('editor.subject')}
        />
      )}
    </div>
  );
}
