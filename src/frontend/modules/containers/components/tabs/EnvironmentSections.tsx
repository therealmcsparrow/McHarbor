// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { InfoRow } from '@resources/components/ui/InfoRow';
import { KeyValueEditor } from '../KeyValueEditor';
import type { ContainerInspect } from '@core/types/docker';
import type { EditFormData } from '../../types/edit-form';
import { EditInput } from './EditFieldControls';
import { ContainerEnvList } from './ContainerEnvList';

type ChangeHandler = <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;

type Translator = (key: string) => string;

export function EnvironmentImageSection({
  t,
  image,
  onFieldChange,
}: {
  t: Translator;
  image: string;
  onFieldChange: ChangeHandler;
}) {
  return (
    <div className="mc-glass rounded-lg p-6 lg:col-span-2">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('environment.image')}
      </h3>
      <EditInput
        label={t('overview.image')}
        value={image}
        onChange={(value) => onFieldChange('image', value)}
        placeholder={t('placeholders.image')}
      />
    </div>
  );
}

export function EnvironmentVariablesSection({
  t,
  containerId,
  editing,
  envEntries,
  onEnvChange,
}: {
  t: Translator;
  containerId: string;
  editing: boolean;
  envEntries: Array<{ key: string; value: string }>;
  onEnvChange: (entries: Array<{ key: string; value: string }>) => void;
}) {
  return (
    <div className="mc-glass rounded-lg p-6 lg:col-span-2">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('environment.envVars')}
      </h3>
      {editing ? (
        <KeyValueEditor
          entries={envEntries}
          onChange={onEnvChange}
          keyLabel={t('edit.key')}
          valueLabel={t('edit.value')}
          addLabel={t('edit.addEnvVar')}
        />
      ) : (
        <ContainerEnvList containerId={containerId} entries={envEntries} />
      )}
    </div>
  );
}

export function EnvironmentConfigSection({
  t,
  container,
  editing,
  editData,
  onFieldChange,
}: {
  t: Translator;
  container: ContainerInspect;
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: ChangeHandler;
}) {
  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('environment.config')}
      </h3>
      {editing ? (
        <div className="space-y-3">
          <EditInput
            label={t('environment.user')}
            value={editData?.user ?? ''}
            onChange={(value) => onFieldChange('user', value)}
            placeholder={t('placeholders.userGroup')}
          />
          <EditInput
            label={t('environment.hostname')}
            value={editData?.hostname ?? ''}
            onChange={(value) => onFieldChange('hostname', value)}
          />
          <EditInput
            label={t('environment.domainname')}
            value={editData?.domainname ?? ''}
            onChange={(value) => onFieldChange('domainname', value)}
          />
          <EditInput
            label={t('environment.workingDir')}
            value={editData?.workingDir ?? ''}
            onChange={(value) => onFieldChange('workingDir', value)}
          />
          <EditInput
            label={t('environment.command')}
            value={editData?.cmd?.join(' ') ?? ''}
            onChange={(value) => onFieldChange('cmd', value ? value.split(' ') : [])}
          />
          <EditInput
            label={t('environment.entrypoint')}
            value={editData?.entrypoint?.join(' ') ?? ''}
            onChange={(value) => onFieldChange('entrypoint', value ? value.split(' ') : [])}
          />
          <EditInput
            label={t('environment.stopSignal')}
            value={editData?.stopSignal ?? ''}
            onChange={(value) => onFieldChange('stopSignal', value)}
            placeholder={t('placeholders.stopSignal')}
          />
        </div>
      ) : (
        <>
          <InfoRow label={t('environment.user')}>{container.Config?.User || '-'}</InfoRow>
          <InfoRow label={t('environment.hostname')}>{container.Config?.Hostname ?? '-'}</InfoRow>
          <InfoRow label={t('environment.domainname')}>{container.Config?.Domainname || '-'}</InfoRow>
          <InfoRow label={t('environment.workingDir')}>{container.Config?.WorkingDir || '/'}</InfoRow>
          <InfoRow label={t('environment.command')}>
            <span className="font-mono text-xs">{container.Config?.Cmd?.join(' ') ?? '-'}</span>
          </InfoRow>
          <InfoRow label={t('environment.entrypoint')}>
            <span className="font-mono text-xs">{container.Config?.Entrypoint?.join(' ') ?? '-'}</span>
          </InfoRow>
          <InfoRow label={t('environment.stopSignal')}>{container.Config?.StopSignal || 'SIGTERM'}</InfoRow>
        </>
      )}
    </div>
  );
}
