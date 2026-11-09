// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PERM } from '@core/auth/permissionKeys';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { SettingsField } from '@resources/components/settings/SettingsGroup';
import { errorDetailsOf } from '@resources/utils/api-mutation';
import { useBackupTempAbout, useBackupTempStatus, useSaveBackupTemp } from '../hooks/useBackupTemp';
import { backupTempRemedy, type BackupTempAbout } from './backupTempRemedy';

const NOT_WRITABLE_CODE = 'err.backup_temp.not_writable';

type Refusal = { summary: string; detail?: string; code?: string };

function CodeBlock({ label, text }: { label: string; text: string }) {
  return (
    <pre
      className="overflow-x-auto rounded-md bg-muted p-2 font-mono text-xs text-foreground"
      // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- a scrollable code block must be reachable by keyboard
      tabIndex={0}
      // oxlint-disable-next-line jsx-a11y/prefer-tag-over-role -- no native tag names a <pre>; the role is what exposes aria-label
      role="group"
      aria-label={label}
    >
      {text}
    </pre>
  );
}

function RemedyBlock({ path, about }: { path: string; about: BackupTempAbout | undefined }) {
  const { t } = useTranslation('containers');
  const remedy = backupTempRemedy(path, about);
  if (remedy.kind === 'container') {
    return <p className="mt-2 text-sm">{t('backupTemp.remedy.container')}</p>;
  }
  if (remedy.kind !== 'sandboxed') return null;
  return (
    <div className="mt-3 space-y-2 text-sm">
      <p>{t('backupTemp.remedy.sandboxedIntro')}</p>
      <p>{t('backupTemp.remedy.step1', { file: remedy.dropInFile })}</p>
      <CodeBlock label={t('backupTemp.remedy.dropInLabel')} text={remedy.dropIn} />
      <p>{t('backupTemp.remedy.reload')}</p>
      <p>{t('backupTemp.remedy.step2')}</p>
      <CodeBlock label={t('backupTemp.remedy.chownLabel')} text={remedy.chown} />
      <p>{t('backupTemp.remedy.again')}</p>
    </div>
  );
}

/**
 * The Location input and its Save. A refused save keeps the typed value and shows the server's
 * message and cause inline; the remedy is built from the typed path, never from the response.
 */
export function BackupTempLocationField() {
  const { t } = useTranslation('containers');
  const { data: temp } = useBackupTempStatus();
  const about = useBackupTempAbout();
  const save = useSaveBackupTemp();
  const [edit, setEdit] = useState<string | null>(null);
  const [refusal, setRefusal] = useState<{ value: string; error: Refusal } | null>(null);

  const envOverride = temp?.envOverride === true;
  const baseline = envOverride ? (temp?.path ?? '') : (temp?.settingPath ?? '');
  const value = edit ?? baseline;
  const typed = value.trim();
  const unchanged = edit === null || typed === baseline;
  const showRefusal = refusal !== null && refusal.value === value;

  function submit(path: string, raw: string) {
    if (save.isPending) return;
    save.mutate(path, {
      onSuccess: () => {
        setEdit(null);
        setRefusal(null);
      },
      onError: (err) => {
        const { summary, detail, code } = errorDetailsOf(err);
        setRefusal({ value: raw, error: { summary, detail, code } });
      },
    });
  }

  function reset() {
    setEdit('');
    submit('', '');
  }

  const alertId = 'backup-temp-location-refusal';
  const noteId = 'backup-temp-location-env-note';
  const describedBy = [showRefusal ? alertId : '', envOverride ? noteId : ''].filter(Boolean).join(' ') || undefined;

  return (
    <SettingsField
      htmlFor="backup-temp-location"
      label={t('backupTemp.location')}
      description={temp ? t('backupTemp.locationHelp', { path: temp.defaultPath }) : undefined}
    >
      <div className="flex flex-wrap items-center gap-2">
        <Input
          id="backup-temp-location"
          className="min-w-0 flex-1 font-mono"
          value={value}
          readOnly={envOverride || save.isPending}
          aria-invalid={showRefusal || undefined}
          aria-describedby={describedBy}
          spellCheck={false}
          autoComplete="off"
          onChange={(e) => {
            setEdit(e.target.value);
            setRefusal(null);
          }}
        />
        {temp?.source === 'setting' && !envOverride && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            permission={PERM.BACKUPS_MANAGE}
            disabled={save.isPending}
            onClick={reset}
          >
            {t('backupTemp.reset')}
          </Button>
        )}
        <Button
          type="button"
          permission={PERM.BACKUPS_MANAGE}
          disabled={save.isPending || envOverride || unchanged}
          onClick={() => submit(typed, value)}
        >
          {save.isPending ? t('backupTemp.saving') : t('actions.save', { ns: 'common' })}
        </Button>
      </div>
      {envOverride && (
        <p id={noteId} className="mt-2 text-sm text-muted-foreground">
          {t('backupTemp.envNote')}
        </p>
      )}
      {showRefusal && refusal && (
        <Alert variant="destructive" title={t('backupTemp.refusedTitle')} className="mt-3">
          <div id={alertId}>
            <p className="text-sm">{refusal.error.summary}</p>
            {refusal.error.detail && <p className="mt-1 font-mono text-xs break-all">{refusal.error.detail}</p>}
            {refusal.error.code === NOT_WRITABLE_CODE && <RemedyBlock path={refusal.value} about={about} />}
          </div>
        </Alert>
      )}
    </SettingsField>
  );
}
