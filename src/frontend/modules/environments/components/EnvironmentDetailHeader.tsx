// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// EnvironmentDetailHeader — the environment's page head.
//
// The identity — icon tile, the name as the page's <h1>, the status indicator
// and the row of mono chips — and the action buttons are portalled into the
// shell's `#pagehead-slot` band, the same place `PageHeader` puts a page's
// description and actions; the hero card below keeps only the tabs. The
// breadcrumb and the back arrow live in the topbar (`HeaderBreadcrumb`), so
// nothing here has to explain where you are — only what you are looking at.

import { useTranslation } from 'react-i18next';
import {
  IconBrandDocker,
  IconCloudComputing,
  IconNetwork,
  IconPencil,
  IconPlugConnected,
  IconRefresh,
  IconServer,
  IconTerminal2,
} from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { Spinner } from '@resources/components/ui/Spinner';
import { PageHeadActions, PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import type { StatusTone } from '@resources/theme/status-palette';
import { truncateId } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import type { EnvironmentInfo } from '../hooks/useEnvironments';

const CONNECTION_ICONS: Record<string, typeof IconServer> = {
  socket: IconServer,
  tcp: IconNetwork,
  tls: IconNetwork,
  ssh: IconTerminal2,
  agent: IconCloudComputing,
};

const STATUS_TONES: Record<string, StatusTone> = {
  connected: 'success',
  disconnected: 'danger',
  error: 'danger',
};

function endpointFor(env: EnvironmentInfo): string | null {
  if (env.connectionType === 'agent') return env.agentHostname ?? null;
  if (env.socketPath) return env.socketPath;
  if (env.host) return `${env.host}:${env.port ?? ''}`;
  return null;
}

type EnvironmentDetailHeaderProps = {
  env: EnvironmentInfo;
  /** Derived connection status: connected / disconnected / error. */
  status: string;
  onTestConnection: () => void;
  testPending: boolean;
  onRegenerateToken: () => void;
  regeneratePending: boolean;
  onEdit: () => void;
  /** Present only on tabs that have something to save. */
  saveLabel?: string;
  onSave?: () => void;
  saveDisabled?: boolean;
  savePending?: boolean;
};

export function EnvironmentDetailHeader({
  env,
  status: connectionStatus,
  onTestConnection,
  testPending,
  onRegenerateToken,
  regeneratePending,
  onEdit,
  saveLabel,
  onSave,
  saveDisabled = false,
  savePending = false,
}: EnvironmentDetailHeaderProps) {
  const { t } = useTranslation('environments');
  const ConnectionIcon = CONNECTION_ICONS[env.connectionType] ?? IconServer;
  const tone = STATUS_TONES[connectionStatus] ?? 'neutral';
  const statusLabel = t(`detail.status.${connectionStatus in STATUS_TONES ? connectionStatus : 'disconnected'}`);
  const endpoint = endpointFor(env);
  const connectionLabel =
    env.connectionType in CONNECTION_ICONS
      ? t(`connectionTypes.${env.connectionType}`)
      : env.connectionType.toUpperCase();

  const identity = (
    <PageHeadIdentity>
      <span
        aria-hidden="true"
        className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
      >
        <ConnectionIcon className="size-6 text-primary" />
      </span>
      <div className="min-w-0">
        <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
          {env.name}
        </h1>
        <div className="mt-1 flex flex-wrap items-center gap-1.5">
          <StatusIndicator tone={tone} label={statusLabel} size="sm" pulse={connectionStatus === 'connected'} />
          {env.isDefault && (
            <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
              {t('badges.default')}
            </Badge>
          )}
          <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
            {connectionLabel}
          </Badge>
          {endpoint && (
            <span className="truncate rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {endpoint}
            </span>
          )}
          {env.dockerVersion && (
            <span className="inline-flex items-center gap-1 rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              <IconBrandDocker className="size-3" aria-hidden="true" />
              {env.dockerVersion}
            </span>
          )}
          <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
            {truncateId(env.id)}
          </span>
        </div>
      </div>
    </PageHeadIdentity>
  );

  const actions = (
    <div className="flex flex-wrap items-center gap-1.5">
      <HeaderActionButton
        tooltip={t('edit.action')}
        permission={PERM.ENVIRONMENTS_MANAGE}
        onClick={onEdit}
        icon={<IconPencil className="size-3.5" />}
      />
      <HeaderActionButton
        tooltip={t('testConnection')}
        permission={PERM.ENVIRONMENTS_MANAGE}
        onClick={onTestConnection}
        disabled={testPending}
        icon={<IconPlugConnected className="size-3.5" />}
      />
      {env.connectionType === 'agent' && (
        <HeaderActionButton
          tooltip={t('detail.regenerateToken')}
          permission={PERM.ENVIRONMENTS_MANAGE}
          onClick={onRegenerateToken}
          disabled={regeneratePending}
          variant="secondary"
          icon={<IconRefresh className="size-3.5" />}
        />
      )}
      {saveLabel && onSave ? (
        <Button permission={PERM.ENVIRONMENTS_MANAGE} size="sm" onClick={onSave} disabled={saveDisabled}>
          {savePending ? <Spinner size="sm" /> : null}
          {saveLabel}
        </Button>
      ) : null}
    </div>
  );

  return (
    <>
      {identity}
      <PageHeadActions>{actions}</PageHeadActions>
    </>
  );
}
