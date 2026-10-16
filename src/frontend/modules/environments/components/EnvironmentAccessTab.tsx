// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Spinner } from '@resources/components/ui/Spinner';
import { useEnvironmentAccess } from '../hooks/useEnvironmentAccess';
import { EnvironmentAccessCredentials } from './EnvironmentAccessCredentials';
import { EnvironmentAccessHostCard } from './EnvironmentAccessHostCard';

type EnvironmentAccessTabProps = {
  envId: string;
};

export function EnvironmentAccessTab({ envId }: EnvironmentAccessTabProps) {
  const { t } = useTranslation('environments');
  const state = useEnvironmentAccess(envId);
  const { access, fields, save, secretOptions } = state;

  if (access.isError) {
    return <ErrorState variant="card" error={access.error} onRetry={() => void access.refetch()} />;
  }

  if (access.isPending) {
    return (
      <div className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
        <Spinner className="h-4 w-4" />
        {t('detail.access.loading')}
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <EnvironmentAccessHostCard state={state} />

      <EnvironmentAccessCredentials
        authType={fields.authType}
        onAuthTypeChange={fields.setAuthType}
        secretId={fields.secretId}
        onSecretIdChange={fields.setSecretId}
        secretOptions={secretOptions}
        sshKey={fields.sshKey}
        onSshKeyChange={fields.setSshKey}
        sshPassword={fields.sshPassword}
        onSshPasswordChange={fields.setSshPassword}
        saving={save.isPending}
        canSave={state.canSave}
        onSave={state.handleSave}
      />
    </div>
  );
}
