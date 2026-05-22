// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconRefresh } from '@tabler/icons-react';
import { StatusBadge, ENVIRONMENT_STATUS } from '@resources/components/ui/StatusBadge';
import { Button } from '@resources/components/ui/Button';

type Environment = {
  connectionType: string;
  socketPath?: string | null;
  host?: string | null;
  port?: number | null;
  dockerVersion?: string | null;
  lastConnected?: string | null;
  agentHostname?: string | null;
  agentOs?: string | null;
  agentArch?: string | null;
  agentVersion?: string | null;
  agentLastSeen?: string | null;
  agentStatus?: string | null;
};

type ConnectionDetailsCardProps = {
  env: Environment;
};

export function ConnectionDetailsCard({ env }: ConnectionDetailsCardProps) {
  const { t } = useTranslation('environments');

  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <h3 className="mb-4 text-sm font-medium text-foreground">{t('detail.connectionDetails')}</h3>
      <div className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2 lg:grid-cols-4">
        <div>
          <span className="text-muted-foreground">{t('detail.type')}</span>
          <p className="font-medium">{env.connectionType.toUpperCase()}</p>
        </div>
        <div>
          <span className="text-muted-foreground">{t('detail.endpoint')}</span>
          <p className="font-mono text-xs font-medium">
            {env.connectionType === 'agent'
              ? (env.agentHostname ?? t('waitingForAgent'))
              : (env.socketPath ?? (env.host ? `${env.host}:${env.port ?? ''}` : '-'))}
          </p>
        </div>
        <div>
          <span className="text-muted-foreground">{t('detail.dockerVersion')}</span>
          <p className="font-medium">{env.dockerVersion ?? '-'}</p>
        </div>
        <div>
          <span className="text-muted-foreground">{t('detail.lastConnected')}</span>
          <p className="font-medium">{env.lastConnected ?? t('detail.never')}</p>
        </div>
      </div>
    </div>
  );
}

type AgentInfoCardProps = {
  env: Environment;
  onRegenerateToken: () => void;
  isRegenerating: boolean;
};

export function AgentInfoCard({ env, onRegenerateToken, isRegenerating }: AgentInfoCardProps) {
  const { t } = useTranslation('environments');

  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <div className="mb-4 flex items-center justify-between">
        <h3 className="text-sm font-medium text-foreground">{t('detail.agentInfo')}</h3>
        <div className="flex items-center gap-2">
          <StatusBadge status={env.agentStatus ?? 'disconnected'} map={ENVIRONMENT_STATUS} />
          <Button
            variant="outline"
            size="sm"
            onClick={onRegenerateToken}
            disabled={isRegenerating}
          >
            <IconRefresh className="mr-1 h-3.5 w-3.5" />
            {t('detail.regenerateToken')}
          </Button>
        </div>
      </div>
      <div className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2 lg:grid-cols-5">
        <div>
          <span className="text-muted-foreground">{t('detail.hostname')}</span>
          <p className="font-medium">{env.agentHostname ?? '-'}</p>
        </div>
        <div>
          <span className="text-muted-foreground">{t('detail.os')}</span>
          <p className="font-medium">{env.agentOs ?? '-'}</p>
        </div>
        <div>
          <span className="text-muted-foreground">{t('detail.architecture')}</span>
          <p className="font-medium">{env.agentArch ?? '-'}</p>
        </div>
        <div>
          <span className="text-muted-foreground">{t('detail.agentVersion')}</span>
          <p className="font-medium">{env.agentVersion ?? '-'}</p>
        </div>
        <div>
          <span className="text-muted-foreground">{t('detail.lastSeen')}</span>
          <p className="font-medium">{env.agentLastSeen ?? '-'}</p>
        </div>
      </div>
    </div>
  );
}
