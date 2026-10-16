// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { StatusError } from '@resources/components/ui/StatusError';
import { useEnvironmentStore } from '@resources/stores/environment';
import { fetchPluginCurrent, usePluginScopes, usePluginSubjects, type PluginKind } from '../hooks/usePluginKinds';
import { PluginSpecField } from './PluginSpecField';

export type PluginSelection = { env: string; subjectId: string; spec: Record<string, string> };

type Props = {
  kind: PluginKind;
  value: PluginSelection;
  onChange: (next: PluginSelection) => void;
};

export function PluginKindFields({ kind, value, onChange }: Props) {
  const { t } = useTranslation('reconciler');
  const environments = useEnvironmentStore((s) => s.environments);
  const scopes = usePluginScopes(kind);
  const subjects = usePluginSubjects(kind, value.env);
  const [loadingCurrent, setLoadingCurrent] = useState(false);
  const [currentError, setCurrentError] = useState('');

  const scopeOptions = kind.ownScopes
    ? (scopes.data ?? []).map((s) => ({ value: s.value, label: s.label, description: s.detail }))
    : environments.map((e) => ({ value: e.id, label: e.name }));

  const fillCurrent = async () => {
    setLoadingCurrent(true);
    setCurrentError('');
    try {
      const current = await fetchPluginCurrent(kind, value.env, value.subjectId);
      onChange({ ...value, spec: { ...current } });
    } catch (err) {
      setCurrentError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoadingCurrent(false);
    }
  };

  return (
    <>
      {kind.description && <p className="text-xs text-muted-foreground">{kind.description}</p>}
      <div>
        <Label className="mb-2">{kind.ownScopes ? t('plugin.cluster') : t('plugin.environment')}</Label>
        <Select
          value={value.env}
          onChange={(env) => onChange({ env, subjectId: '', spec: {} })}
          ariaLabel={kind.ownScopes ? t('plugin.cluster') : t('plugin.environment')}
          placeholder={t('pick.select')}
          options={scopeOptions}
        />
      </div>
      <div>
        <Label className="mb-2">{kind.subjectLabel}</Label>
        <Select
          value={value.subjectId}
          onChange={(subjectId) => onChange({ ...value, subjectId, spec: {} })}
          ariaLabel={kind.subjectLabel}
          placeholder={subjects.isLoading ? t('pick.loading') : t('pick.select')}
          searchable
          disabled={value.env === ''}
          options={(subjects.data ?? []).map((s) => ({ value: s.value, label: s.label, description: s.detail }))}
        />
        {subjects.isError && <StatusError error={subjects.error instanceof Error ? subjects.error.message : ''} />}
      </div>
      {value.subjectId !== '' && (
        <div className="space-y-3 rounded-md border border-border p-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-xs text-muted-foreground">{t('plugin.pinHint')}</p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={loadingCurrent}
              onClick={() => void fillCurrent()}
            >
              {t('plugin.useCurrent')}
            </Button>
          </div>
          {currentError && <StatusError error={currentError} />}
          {kind.fields.map((field) => (
            <PluginSpecField
              key={field.key}
              kind={kind}
              field={field}
              env={value.env}
              value={value.spec[field.key] ?? ''}
              onChange={(v) => onChange({ ...value, spec: { ...value.spec, [field.key]: v } })}
            />
          ))}
        </div>
      )}
    </>
  );
}
