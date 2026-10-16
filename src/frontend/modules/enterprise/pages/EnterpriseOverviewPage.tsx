// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  IconActivity,
  IconArchive,
  IconBrandOauth,
  IconCertificate,
  IconCloudLock,
  IconDatabase,
  IconFileExport,
  IconFolders,
  IconKey,
  IconPlugConnected,
  IconShieldCheck,
  IconStethoscope,
  IconUsersGroup,
} from '@tabler/icons-react';
import type { TablerIcon } from '@tabler/icons-react';
import { api, type PaginatedData } from '@core/api/client';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Box, Flex, Grid, Stack } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { PageHeader } from '@resources/layout/PageHeader';
import { HAMigrationWizard } from '../components/HAMigrationWizard';
import { assertSuccess } from '@resources/utils/api-mutation';

type ClusterStatus = {
  nodeId: string;
  databaseDriver: string;
  roles: Record<string, boolean>;
  database: { reachable: boolean };
  role?: string;
  mode?: string;
};
type Countable = { length: number };
type AuditPage = PaginatedData<Record<string, unknown>>;
type BackupLogPage = { total: number };
type StatusTone = 'success' | 'warning' | 'secondary';

type StatusCard = {
  id: string;
  icon: TablerIcon;
  to: string;
  value: string;
  tone: StatusTone;
};

const supportCards = [
  { id: 'bundle', to: '/enterprise/support-bundle', icon: IconArchive },
  { id: 'diagnostics', to: '/enterprise/diagnostics', icon: IconStethoscope },
  { id: 'inventory', to: '/enterprise/state-inventory', icon: IconFolders },
];

const linkCards = [
  { id: 'security', to: '/security', icon: IconShieldCheck },
  { id: 'identity', to: '/security', icon: IconBrandOauth },
  { id: 'scim', to: '/security', icon: IconUsersGroup },
  { id: 'policies', to: '/security/policies', icon: IconShieldCheck },
  { id: 'audit', to: '/audit', icon: IconActivity },
  { id: 'auditExport', to: '/audit', icon: IconFileExport },
  { id: 'siem', to: '/settings?tab=logging', icon: IconCloudLock },
  { id: 'apiKeys', to: '/security', icon: IconKey },
  { id: 'backups', to: '/backups', icon: IconArchive },
  { id: 'storage', to: '/settings', icon: IconDatabase },
  { id: 'plugins', to: '/store', icon: IconPlugConnected },
  { id: 'ha', to: '/enterprise/ha', icon: IconCloudLock },
  { id: 'license', to: '/settings?tab=license', icon: IconCertificate },
];

export default function EnterpriseOverviewPage() {
  const { t } = useTranslation('enterprise');
  const cluster = useEnterpriseQuery<ClusterStatus>('cluster-status', '/cluster/status');
  const identity = useEnterpriseQuery<Countable>('identityProviders', '/identity-providers');
  const scim = useEnterpriseQuery<Countable>('scim-endpoints', '/scim/endpoints');
  const apiKeys = useEnterpriseQuery<Countable>('api-keys', '/api-keys');
  const storage = useEnterpriseQuery<Countable>('storageLocations', '/storage-locations');
  const plugins = useEnterpriseQuery<Countable>('plugin-status', '/plugins/loader/status');
  // Audit forwarding is a plugin family now: there is no single SIEM
  // config to read, so the card reports whether *any* installed
  // forwarder is enabled. Zero installed and zero enabled are
  // deliberately the same warning state — both mean the audit trail
  // is not leaving this host.
  const forwarders = useEnterpriseQuery<{ enabled: boolean }[]>('audit-log-forwarders', '/audit/logging');
  const audit = useQuery({
    queryKey: ['enterprise', 'audit'],
    queryFn: () => api.get<AuditPage>('/audit', { page: '1', per_page: '1' }).then(assertSuccess),
  });
  const backups = useQuery({
    queryKey: ['enterprise', 'backup-logs'],
    queryFn: () => api.get<BackupLogPage>('/backup-logs', { page: '1', perPage: '1' }).then(assertSuccess),
  });

  const statusCards: StatusCard[] = [
    {
      id: 'ha',
      icon: IconCloudLock,
      to: '/enterprise/ha',
      value: cluster.data ? `${cluster.data.databaseDriver} · ${cluster.data.role ?? 'peer'}` : valueFor(cluster),
      tone: cluster.data?.database.reachable ? 'success' : cluster.isError ? 'warning' : 'secondary',
    },
    {
      id: 'identity',
      icon: IconBrandOauth,
      to: '/security',
      value: countValue(identity),
      tone: countTone(identity.data?.length),
    },
    { id: 'scim', icon: IconUsersGroup, to: '/security', value: countValue(scim), tone: countTone(scim.data?.length) },
    {
      id: 'audit',
      icon: IconActivity,
      to: '/audit',
      value: totalValue(audit.data?.total, audit.isLoading),
      tone: audit.data ? 'success' : 'secondary',
    },
    {
      id: 'auditExport',
      icon: IconFileExport,
      to: '/audit',
      value: audit.data ? t('values.ready') : valueFor(audit),
      tone: audit.data ? 'success' : 'secondary',
    },
    {
      id: 'siem',
      icon: IconCloudLock,
      to: '/settings?tab=logging',
      value: forwarders.isLoading
        ? '-'
        : forwarders.data?.some((f) => f.enabled)
          ? t('values.enabled')
          : t('values.disabled'),
      tone: forwarders.data?.some((f) => f.enabled) ? 'success' : 'warning',
    },
    {
      id: 'backups',
      icon: IconArchive,
      to: '/backups',
      value: totalValue(backups.data?.total, backups.isLoading),
      tone: backups.data ? 'success' : 'secondary',
    },
    {
      id: 'storage',
      icon: IconDatabase,
      to: '/settings',
      value: countValue(storage),
      tone: countTone(storage.data?.length),
    },
    {
      id: 'plugins',
      icon: IconPlugConnected,
      to: '/store',
      value: countValue(plugins),
      tone: countTone(plugins.data?.length),
    },
    {
      id: 'apiKeys',
      icon: IconKey,
      to: '/security',
      value: countValue(apiKeys),
      tone: countTone(apiKeys.data?.length),
    },
  ];

  return (
    <div className="space-y-6">
      <PageHeader title={t('title')} description={t('description')} headingLeaf />
      <Grid columns="grid-cols-1 md:grid-cols-2 xl:grid-cols-4" gap={4}>
        {statusCards.map((card) => (
          <EnterpriseStatusCard key={card.id} card={card} />
        ))}
      </Grid>
      <HAMigrationWizard />
      <Grid columns="grid-cols-1 xl:grid-cols-[1.4fr_0.8fr]" gap={6}>
        <Card>
          <CardHeader>
            <CardTitle>{t('sections.surfaces')}</CardTitle>
          </CardHeader>
          <CardContent>
            <Grid columns="grid-cols-1 md:grid-cols-2" gap={3}>
              {linkCards.map((card) => (
                <SurfaceLink key={card.id} {...card} />
              ))}
            </Grid>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t('overview.section', { ns: 'support' })}</CardTitle>
          </CardHeader>
          <CardContent>
            <Stack gap={3}>
              {supportCards.map((card) => (
                <SupportLink key={card.id} {...card} />
              ))}
            </Stack>
          </CardContent>
        </Card>
      </Grid>
    </div>
  );
}

function useEnterpriseQuery<T>(key: string, path: string) {
  return useQuery({ queryKey: ['enterprise', key], queryFn: () => api.get<T>(path).then(assertSuccess), retry: false });
}

function EnterpriseStatusCard({ card }: { card: StatusCard }) {
  const { t } = useTranslation('enterprise');
  const Icon = card.icon;
  return (
    <Card>
      <CardContent className="space-y-4">
        <Flex align="center" justify="between" gap={3}>
          <Box className="rounded-lg border border-border bg-muted/40 p-2">
            <Icon className="size-5 text-primary" />
          </Box>
          <Badge variant={card.tone}>{t(`status.${card.tone}`)}</Badge>
        </Flex>
        <Stack gap={1}>
          <TypographyText weight="semibold">{t(`cards.${card.id}.title`)}</TypographyText>
          <TypographyText tone="muted">{t(`cards.${card.id}.description`)}</TypographyText>
          <TypographyText size="xl" weight="bold">
            {card.value}
          </TypographyText>
        </Stack>
        <Button variant="outline" size="sm" asChild>
          <Link to={card.to}>{t('actions.open')}</Link>
        </Button>
      </CardContent>
    </Card>
  );
}

function SurfaceLink({ id, to, icon: Icon }: { id: string; to: string; icon: TablerIcon }) {
  const { t } = useTranslation('enterprise');
  return (
    <Button variant="outline" className="h-auto justify-start p-4" asChild>
      <Link to={to}>
        <Icon className="size-5" />
        <Stack gap={1}>
          <TypographyText weight="medium">{t(`links.${id}.title`)}</TypographyText>
          <TypographyText tone="muted">{t(`links.${id}.description`)}</TypographyText>
        </Stack>
      </Link>
    </Button>
  );
}

function SupportLink({ id, to, icon: Icon }: { id: string; to: string; icon: TablerIcon }) {
  const { t } = useTranslation('support');
  return (
    <Button variant="outline" className="h-auto justify-start p-4" asChild>
      <Link to={to}>
        <Icon className="size-5" />
        <Stack gap={1}>
          <TypographyText weight="medium">{t(`overview.${id}.title`)}</TypographyText>
          <TypographyText tone="muted">{t(`overview.${id}.description`)}</TypographyText>
        </Stack>
      </Link>
    </Button>
  );
}

function countValue(query: { data?: Countable; isLoading: boolean }) {
  return query.isLoading ? '-' : String(query.data?.length ?? 0);
}
function totalValue(total: number | undefined, loading: boolean) {
  return loading ? '-' : String(total ?? 0);
}
function valueFor(query: { isLoading: boolean }) {
  return query.isLoading ? '-' : 'unknown';
}
function countTone(count: number | undefined): StatusTone {
  return count && count > 0 ? 'success' : 'warning';
}
