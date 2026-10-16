// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { formatBytes, formatDate } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { useStartupHealth } from '../hooks/useCluster';

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 py-2">
      <TypographyText size="sm" tone="muted">
        {label}
      </TypographyText>
      <div className="flex items-center gap-2">{children}</div>
    </div>
  );
}

function Flag({ ok, yes, no }: { ok: boolean; yes: string; no: string }) {
  return <Badge variant={ok ? 'success' : 'destructive'}>{ok ? yes : no}</Badge>;
}

export function StartupHealthCard() {
  const { t } = useTranslation('enterprise');
  const formatPrefs = useLocaleFormat();
  const canView = usePermission(PERM.CLUSTER_VIEW);
  const health = useStartupHealth(canView);
  const r = health.data;
  if (!canView || !r) return null;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('ha.startup.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <TypographyText tone="muted" size="sm">
          {t('ha.startup.description', { time: formatDate(r.checkedAt, formatPrefs) })}
        </TypographyText>
        {r.haModeRefused && <Alert variant="warning">{t('ha.startup.haModeRefused')}</Alert>}
        <div className="divide-y divide-border">
          <Row label={t('ha.startup.database')}>
            <Badge variant="outline" className="font-mono">
              {r.databaseDriver}
            </Badge>
            <Flag ok={r.databaseReachable} yes={t('ha.reachable')} no={t('ha.unreachable')} />
          </Row>
          <Row label={t('ha.startup.dataDir')}>
            <Flag ok={r.dataDirWritable} yes={t('ha.startup.writable')} no={t('ha.startup.notWritable')} />
            <Badge variant={r.lowDisk ? 'warning' : 'secondary'}>
              {t('ha.startup.free', { size: formatBytes(r.dataDirFreeBytes) })}
            </Badge>
          </Row>
          <Row label={t('ha.startup.keySource')}>
            <Badge variant="secondary">{t(`ha.startup.key.${r.encryptionKeySource}`)}</Badge>
          </Row>
          <Row label={t('ha.startup.tls')}>
            <Badge variant="secondary">{t(`ha.startup.tlsMode.${r.tlsMode}`)}</Badge>
          </Row>
          <Row label={t('ha.facts.pluginStore')}>
            <Badge variant="secondary">{t(`ha.pluginStore.${r.pluginStore}`)}</Badge>
          </Row>
          <Row label={t('ha.facts.mode')}>
            <Badge variant="secondary">{t(`ha.mode.${r.haMode}`)}</Badge>
          </Row>
        </div>
      </CardContent>
    </Card>
  );
}
