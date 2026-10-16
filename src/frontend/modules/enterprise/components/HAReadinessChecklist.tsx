// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCheck, IconX } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Spinner } from '@resources/components/ui/Spinner';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import {
  type GateStatus,
  type ReadinessGate,
  type ReadinessTarget,
  useAgentRouting,
  useAgentRoutingCheck,
  useClusterReadiness,
} from '../hooks/useCluster';

const TARGETS: ReadinessTarget[] = ['active-active', 'active-passive'];

const BADGE: Record<GateStatus, 'success' | 'warning' | 'destructive'> = {
  pass: 'success',
  warn: 'warning',
  fail: 'destructive',
};

function GateIcon({ status: statusValue }: { status: GateStatus }) {
  if (statusValue === 'pass') return <IconCheck className="size-4 shrink-0 text-success" aria-hidden="true" />;
  if (statusValue === 'warn') return <IconAlertTriangle className="size-4 shrink-0 text-warning" aria-hidden="true" />;
  return <IconX className="size-4 shrink-0 text-destructive" aria-hidden="true" />;
}

function GateRow({ gate }: { gate: ReadinessGate }) {
  const { t } = useTranslation('enterprise');
  return (
    <li className="flex items-start gap-3 py-3">
      <GateIcon status={gate.status} />
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <TypographyText size="sm" weight="semibold">
            {t(`ha.readiness.gate.${gate.id}.title`)}
          </TypographyText>
          <Badge variant={BADGE[gate.status]}>{t(`ha.readiness.status.${gate.status}`)}</Badge>
          {(gate.values ?? []).filter(Boolean).map((v) => (
            <Badge key={v} variant="outline" className="font-mono">
              {v}
            </Badge>
          ))}
        </div>
        <TypographyText size="sm" tone="muted">
          {t(`ha.readiness.gate.${gate.id}.${gate.status}`)}
        </TypographyText>
      </div>
    </li>
  );
}

function RoutingProof() {
  const { t } = useTranslation('enterprise');
  const canView = usePermission(PERM.CLUSTER_VIEW);
  const routing = useAgentRouting(canView);
  const check = useAgentRoutingCheck();
  const proof = routing.data?.proof;
  const owners = routing.data?.owners ?? [];
  const elsewhere = owners.filter((o) => o.nodeId !== routing.data?.nodeId);

  return (
    <div className="flex flex-wrap items-center gap-3 rounded-md border border-border bg-muted/40 p-3">
      <TypographyText size="sm" tone="muted" className="min-w-0 flex-1">
        {proof?.status === 'proven'
          ? t('ha.readiness.routing.proven', { node: proof.nodeId ?? '', env: proof.envId ?? '' })
          : proof?.status === 'failed'
            ? t('ha.readiness.routing.failed', { error: proof.error ?? '' })
            : elsewhere.length === 0
              ? t('ha.readiness.routing.nothingToRoute')
              : t('ha.readiness.routing.unproven', { count: elsewhere.length, nodes: elsewhere.length })}
      </TypographyText>
      <Button
        size="sm"
        variant="outline"
        permission={PERM.CLUSTER_MANAGE}
        disabled={check.isPending}
        onClick={() => check.mutate(undefined)}
      >
        {t('ha.readiness.routing.check')}
      </Button>
    </div>
  );
}

export function HAReadinessChecklist() {
  const { t } = useTranslation('enterprise');
  const canView = usePermission(PERM.CLUSTER_VIEW);
  const [target, setTarget] = useState<ReadinessTarget>('active-active');
  const readiness = useClusterReadiness(target, canView);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('ha.readiness.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <TypographyText tone="muted" size="sm">
          {t('ha.readiness.description')}
        </TypographyText>
        <fieldset className="flex min-w-0 flex-wrap gap-2" aria-label={t('ha.readiness.targetLabel')}>
          {TARGETS.map((value) => (
            <Button
              key={value}
              size="sm"
              variant={target === value ? 'default' : 'outline'}
              aria-pressed={target === value}
              permission={PERM.CLUSTER_VIEW}
              onClick={() => setTarget(value)}
            >
              {t(`ha.readiness.target.${value}`)}
            </Button>
          ))}
        </fieldset>
        {!canView ? (
          <Alert variant="warning">{t('permission.requires', { ns: 'common', permission: PERM.CLUSTER_VIEW })}</Alert>
        ) : readiness.isLoading ? (
          <Spinner />
        ) : readiness.error ? (
          <Alert variant="destructive">{readiness.error.message}</Alert>
        ) : readiness.data ? (
          <>
            <Alert variant={readiness.data.ready ? 'success' : 'warning'}>
              {t(readiness.data.ready ? 'ha.readiness.verdict.ready' : 'ha.readiness.verdict.notReady')}
              {readiness.data.target === 'active-active' && !readiness.data.enabled && (
                <span className="mt-1 block">{t('ha.readiness.activeActiveDisabled')}</span>
              )}
            </Alert>
            {readiness.data.target === 'active-active' && <RoutingProof />}
            <ul className="divide-y divide-border">
              {readiness.data.gates.map((gate) => (
                <GateRow key={gate.id} gate={gate} />
              ))}
            </ul>
          </>
        ) : null}
      </CardContent>
    </Card>
  );
}
