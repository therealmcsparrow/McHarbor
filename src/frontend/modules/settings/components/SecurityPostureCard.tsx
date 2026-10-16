// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconShieldCheck, IconShieldExclamation } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { useSecurityPosture, type PostureFinding } from '../hooks/useSecurityPosture';

const FINDING_KEYS: Record<string, { title: string; fix: string }> = {
  AUTH_DISABLE: { title: 'about.posture.findings.authDisabled.title', fix: 'about.posture.findings.authDisabled.fix' },
  TLS_ENABLED: { title: 'about.posture.findings.tlsLocalCa.title', fix: 'about.posture.findings.tlsLocalCa.fix' },
  HSTS_MAX_AGE: { title: 'about.posture.findings.hstsOff.title', fix: 'about.posture.findings.hstsOff.fix' },
  ALLOWED_ORIGINS: {
    title: 'about.posture.findings.allowedOriginsEmpty.title',
    fix: 'about.posture.findings.allowedOriginsEmpty.fix',
  },
  MCHARBOR_PLUGIN_REQUIRE_SIGNATURE: {
    title: 'about.posture.findings.pluginSignatureOptional.title',
    fix: 'about.posture.findings.pluginSignatureOptional.fix',
  },
};

const SEVERITY_KEYS = {
  critical: 'about.posture.severity.critical',
  warning: 'about.posture.severity.warning',
} as const;

function FindingRow({ finding }: { finding: PostureFinding }) {
  const { t } = useTranslation('settings');
  const keys = FINDING_KEYS[finding.envVar];
  const critical = finding.severity === 'critical';
  return (
    <li className="flex flex-col gap-1.5 py-3 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={critical ? 'destructive' : 'warning'}>
          {t(critical ? SEVERITY_KEYS.critical : SEVERITY_KEYS.warning)}
        </Badge>
        <span className="text-sm font-medium text-foreground">{keys ? t(keys.title) : finding.message}</span>
      </div>
      {keys && <p className="text-sm text-muted-foreground">{t(keys.fix)}</p>}
      <p className="text-xs text-muted-foreground">
        {t('about.posture.envVar')}: <code className="rounded bg-muted px-1.5 py-0.5 font-mono">{finding.envVar}</code>
      </p>
    </li>
  );
}

function PostureBody({ enabled }: { enabled: boolean }) {
  const { t } = useTranslation('settings');
  const { data, isLoading, isError } = useSecurityPosture(enabled);

  if (!enabled) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('permission.requires', { ns: 'common', permission: PERM.SETTINGS_VIEW })}
      </p>
    );
  }
  if (isLoading) {
    return (
      <div className="space-y-2" aria-busy="true">
        <Skeleton className="h-4 w-2/3" />
        <Skeleton className="h-3 w-full" />
      </div>
    );
  }
  if (isError || !data) {
    return <p className="text-sm text-destructive">{t('about.posture.loadFailed')}</p>;
  }
  if (data.findings.length === 0) {
    return (
      <p className="flex items-center gap-2 text-sm text-foreground">
        <IconShieldCheck className="size-4 text-success" aria-hidden="true" />
        {t('about.posture.allClear')}
      </p>
    );
  }
  return (
    <ul className="divide-y divide-border">
      {data.findings.map((finding) => (
        <FindingRow key={finding.envVar} finding={finding} />
      ))}
    </ul>
  );
}

export function SecurityPostureCard() {
  const { t } = useTranslation('settings');
  const canView = usePermission(PERM.SETTINGS_VIEW);
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-sm">
          <IconShieldExclamation className="size-4 text-muted-foreground" aria-hidden="true" />
          {t('about.posture.title')}
        </CardTitle>
        <CardDescription className="mt-1">{t('about.posture.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        <PostureBody enabled={canView} />
      </CardContent>
    </Card>
  );
}
