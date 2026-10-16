// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconArrowRight } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import {
  useMigrateConnection,
  type ConnectionTarget,
  type MigrateConnectionData,
} from '../hooks/useEnvironmentMigration';
import { MigrateAgentFields } from './MigrateAgentFields';
import { MigrateEndpointFields, type MigrateEndpointValues } from './MigrateEndpointFields';

type MigrateConnectionDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  environment: {
    id: string;
    name: string;
    connectionType: string;
    socketPath?: string | null;
    host?: string | null;
    port?: number | null;
  };
  onAgentToken: (token: string) => void;
};

export function MigrateConnectionDialog({
  open: isOpen,
  onOpenChange,
  environment,
  onAgentToken,
}: MigrateConnectionDialogProps) {
  const { t } = useTranslation('environments');
  const migrate = useMigrateConnection();

  const targets = useMemo<ConnectionTarget[]>(
    () => (['agent', 'socket'] as ConnectionTarget[]).filter((entry) => entry !== environment.connectionType),
    [environment.connectionType],
  );

  const [target, setTarget] = useState<ConnectionTarget>(targets[0] ?? 'agent');
  // docker socket: default value of the field the operator types into, used only when the server sent none.
  const [endpoint, setEndpoint] = useState<MigrateEndpointValues>({
    socketPath: environment.socketPath ?? '/var/run/docker.sock',
    host: environment.host ?? '',
    port: environment.port ?? 2375,
    sshHost: '',
    sshPort: 22,
    sshUser: 'root',
  });
  const updateEndpoint = (patch: Partial<MigrateEndpointValues>) =>
    setEndpoint((current) => ({ ...current, ...patch }));
  const { socketPath, host, port, sshHost, sshPort, sshUser } = endpoint;
  const [agentToken, setAgentToken] = useState('');
  const [immediate, setImmediate] = useState(false);
  const [deployAgent, setDeployAgent] = useState(true);
  const [insecureTls, setInsecureTls] = useState(
    typeof window !== 'undefined' && window.location.protocol === 'https:',
  );
  const [deployError, setDeployError] = useState<string | null>(null);

  const canSubmit =
    target === 'agent'
      ? true
      : target === 'socket'
        ? socketPath.trim().length > 0
        : target === 'tcp'
          ? host.trim().length > 0 && port > 0
          : sshHost.trim().length > 0 && sshUser.trim().length > 0;

  const submit = () => {
    if (!canSubmit) return;
    const data: MigrateConnectionData = { targetType: target };
    if (target === 'agent') {
      if (agentToken.trim()) data.agentToken = agentToken.trim();
      data.immediate = immediate;
      data.skipAgentDeploy = !deployAgent;
      data.insecureTls = insecureTls;
    } else if (target === 'socket') {
      data.socketPath = socketPath.trim();
    } else if (target === 'tcp') {
      data.host = host.trim();
      data.port = port;
    } else {
      data.sshHost = sshHost.trim();
      data.sshPort = sshPort || 22;
      data.sshUser = sshUser.trim();
    }

    setDeployError(null);
    migrate.mutate(
      { id: environment.id, data },
      {
        onSuccess: (result) => {
          if (result.agent?.attempted && !result.agent.deployed) {
            setDeployError(result.agent.error ?? t('migrate.deployFailed'));
            if (result.agentToken) onAgentToken(result.agentToken);
            return;
          }
          if (result.agentToken && !result.agent?.deployed) onAgentToken(result.agentToken);
          onOpenChange(false);
        },
      },
    );
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('migrate.title')}</DialogTitle>
          <DialogDescription>{t('migrate.description')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="flex items-center gap-3 rounded-lg border border-border bg-card p-4 text-sm">
            <span className="font-medium text-foreground">{t(`connectionTypes.${environment.connectionType}`)}</span>
            <IconArrowRight className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
            <span className="font-medium text-foreground">{t(`connectionTypes.${target}`)}</span>
          </div>

          <div>
            <Label className="mb-2">{t('migrate.targetType')}</Label>
            <Select
              value={target}
              ariaLabel={t('migrate.targetType')}
              options={targets.map((entry) => ({
                value: entry,
                label: t(`connectionTypes.${entry}`),
              }))}
              onChange={(value) => setTarget(value as ConnectionTarget)}
            />
          </div>

          {target === 'agent' ? (
            <MigrateAgentFields
              deployAgent={deployAgent}
              onDeployAgentChange={setDeployAgent}
              insecureTls={insecureTls}
              onInsecureTlsChange={setInsecureTls}
              agentToken={agentToken}
              onAgentTokenChange={setAgentToken}
              immediate={immediate}
              onImmediateChange={setImmediate}
            />
          ) : (
            <MigrateEndpointFields target={target} values={endpoint} onChange={updateEndpoint} />
          )}

          {deployError && (
            <div className="flex items-start gap-2 rounded-md bg-destructive/10 p-3">
              <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" aria-hidden="true" />
              <div>
                <p className="text-sm font-medium text-foreground">{t('migrate.deployFailed')}</p>
                <p className="mt-1 font-mono text-xs text-muted-foreground">{deployError}</p>
                <p className="mt-1 text-xs text-muted-foreground">{t('migrate.deployFailedHint')}</p>
              </div>
            </div>
          )}

          {environment.connectionType === 'agent' && (
            <div className="flex items-start gap-2 rounded-md bg-warning/10 p-3">
              <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
              <p className="text-sm text-foreground">{t('migrate.revokeWarning')}</p>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={PERM.ENVIRONMENTS_MANAGE} onClick={submit} disabled={!canSubmit || migrate.isPending}>
            {migrate.isPending && <Spinner className="mr-2 h-4 w-4" />}
            {t('migrate.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
