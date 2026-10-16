// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { PluginLink as Link } from '../nav-context';
import { useTranslation } from 'react-i18next';
import { IconSettings } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent } from '@resources/components/ui/Card';
import { statusSoft } from '@resources/theme/status-palette';
import { useSwarmCapability } from '../hooks/useSwarmCapability';
import { shortId } from '../format';

/**
 * What this node is, before anything about what runs on it.
 *
 * Rendered from the capability probe rather than the cluster read, so a worker
 * — which cannot inspect the cluster at all — still gets an honest answer
 * instead of an error card.
 */
export function ClusterIdentityCard() {
  const { t } = useTranslation('swarm');
  const { capability } = useSwarmCapability();

  return (
    <Card>
      <CardContent className="flex flex-wrap items-center gap-x-8 gap-y-4 p-5">
        <Detail label={t('overview.identity.role')}>
          <Badge variant={capability.controlAvailable ? 'default' : 'secondary'}>
            {t(`nodeRole.${capability.nodeRole || 'unknown'}`)}
          </Badge>
        </Detail>
        <Detail label={t('overview.identity.clusterId')}>
          <code className="font-mono text-sm text-foreground">
            {capability.clusterId ? shortId(capability.clusterId) : '—'}
          </code>
        </Detail>
        <Detail label={t('overview.identity.nodeId')}>
          <code className="font-mono text-sm text-foreground">
            {capability.nodeId ? shortId(capability.nodeId) : '—'}
          </code>
        </Detail>
        <Detail label={t('overview.identity.managers')}>
          <span className="text-sm text-foreground">
            {capability.managers}/{capability.nodes}
          </span>
        </Detail>
        <div className="ms-auto">
          <Button variant="ghost" size="sm" asChild>
            <Link to="nodes">
              <IconSettings className="size-4" />
              {t('overview.identity.manageNodes')}
            </Link>
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function Detail({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1">
      <p className="text-xs tracking-wide text-muted-foreground uppercase">{label}</p>
      {children}
    </div>
  );
}

export function StatCard({
  to,
  icon,
  label,
  value,
  detail,
  tone,
}: {
  to: string;
  icon: React.ReactNode;
  label: string;
  value: string;
  detail: string;
  tone: 'success' | 'warning' | 'danger' | 'info';
}) {
  return (
    <Link to={to} className="block rounded-xl transition-colors hover:bg-muted/40">
      <Card className="h-full border-0 bg-transparent shadow-none">
        <CardContent className="space-y-3 p-5">
          <div className="flex items-center justify-between">
            <span className="text-sm text-muted-foreground">{label}</span>
            <span className={`rounded-lg p-1.5 ${statusSoft(tone)}`}>{icon}</span>
          </div>
          <p className="text-2xl font-semibold text-foreground">{value}</p>
          <p className="text-xs text-muted-foreground">{detail}</p>
        </CardContent>
      </Card>
    </Link>
  );
}

export function SmallStat({ label, value, to }: { label: string; value: string; to: string }) {
  return (
    <Link
      to={to}
      className="flex items-center justify-between rounded-xl border border-border px-4 py-3 transition-colors hover:bg-muted/40"
    >
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="text-lg font-semibold text-foreground">{value}</span>
    </Link>
  );
}
