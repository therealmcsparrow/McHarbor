// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconDownload } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { Switch } from '@resources/components/ui/Switch';
import { useEnvironmentList } from '@resources/hooks/useEnvironmentList';
import { PERM } from '@core/auth/permissionKeys';
import { ComplianceAttestationPanel } from '@resources/components/compliance/ComplianceAttestationPanel';
import { ACCESS_REVIEW_FRAMEWORK } from '@resources/hooks/useComplianceAttestations';
import { useUsers } from '@resources/hooks/useUsers';
import { useEffectivePermissions, useExportPermissionCatalog } from '../../hooks/useAccessReview';
import { EffectivePermissionTable } from './EffectivePermissionTable';

const GLOBAL = '__global__';
const WHOLE = '__whole__';
const EXPORT_FORMATS = ['csv', 'json'] as const;

export function EffectiveAccessTab() {
  const { t } = useTranslation('access');
  const [userId, setUserId] = useState('');
  const [environmentId, setEnvironmentId] = useState('');
  const [stack, setStack] = useState('');
  const [filter, setFilter] = useState('');
  const [showAll, setShowAll] = useState(false);
  const users = useUsers();
  const environments = useEnvironmentList();
  const effective = useEffectivePermissions({ userId, environmentId, stack });
  const exportCatalog = useExportPermissionCatalog();

  const userOptions = useMemo(
    () =>
      (users.data ?? []).map((u) => ({
        value: u.id,
        label: u.displayName ? `${u.displayName} (${u.username})` : u.username,
      })),
    [users.data],
  );
  const environmentOptions = useMemo(
    () => [
      { value: GLOBAL, label: t('effective.allEnvironments') },
      ...(environments.data ?? []).map((e) => ({ value: e.id, label: e.name })),
    ],
    [environments.data, t],
  );
  const stackOptions = useMemo(
    () => [
      { value: WHOLE, label: t('effective.noStack') },
      ...(effective.data?.stackScopes ?? []).map((s) => ({ value: s, label: s })),
    ],
    [effective.data?.stackScopes, t],
  );
  const data = effective.data;
  const held = data?.permissions.filter((p) => p.effective).length ?? 0;

  return (
    <section className="mc-glass space-y-4 rounded-lg p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold text-foreground">{t('effective.title')}</h2>
          <p className="text-sm text-muted-foreground">{t('effective.description')}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {EXPORT_FORMATS.map((format) => (
            <Button
              key={format}
              variant="outline"
              size="sm"
              permission={PERM.PERMISSIONS_VIEW}
              disabled={exportCatalog.isPending}
              onClick={() => exportCatalog.mutate(format)}
            >
              <IconDownload className="size-4" />
              {format === 'csv' ? t('catalog.exportCsv') : t('catalog.exportJson')}
            </Button>
          ))}
        </div>
      </div>
      {exportCatalog.isError && <Alert variant="destructive">{exportCatalog.error.message}</Alert>}

      <ComplianceAttestationPanel
        framework={ACCESS_REVIEW_FRAMEWORK}
        frameworkName={t('attestation.title')}
        title={t('attestation.title')}
        description={t('attestation.description')}
      />

      <div className="grid gap-3 md:grid-cols-3">
        <div>
          <Label className="mb-1">{t('effective.user')}</Label>
          <Select
            value={userId}
            onChange={(v) => {
              setUserId(v);
              setStack('');
            }}
            options={userOptions}
            placeholder={t('effective.userPlaceholder')}
            searchable
            ariaLabel={t('effective.user')}
          />
        </div>
        <div>
          <Label className="mb-1">{t('effective.environment')}</Label>
          <Select
            value={environmentId || GLOBAL}
            onChange={(v) => {
              setEnvironmentId(v === GLOBAL ? '' : v);
              setStack('');
            }}
            options={environmentOptions}
            ariaLabel={t('effective.environment')}
          />
        </div>
        <div>
          <Label className="mb-1">{t('effective.stack')}</Label>
          <Select
            value={stack || WHOLE}
            onChange={(v) => setStack(v === WHOLE ? '' : v)}
            options={stackOptions}
            disabled={!environmentId}
            ariaLabel={t('effective.stack')}
          />
        </div>
      </div>

      {userId === '' && <p className="py-6 text-center text-sm text-muted-foreground">{t('effective.empty')}</p>}
      {effective.isLoading && <Spinner />}
      {effective.isError && <Alert variant="destructive">{effective.error.message}</Alert>}
      {data && (
        <div className="space-y-3">
          {data.wildcard && <Alert variant="warning">{t('effective.wildcard', { username: data.username })}</Alert>}
          {data.breakGlass && <Alert variant="warning">{t('effective.breakGlass', { username: data.username })}</Alert>}
          {data.abacEnforced && <Alert variant="info">{t('effective.abac')}</Alert>}
          <p className="text-xs text-muted-foreground">{t('effective.overridesNote')}</p>
          <div className="flex flex-wrap items-center gap-3">
            <Input
              variant="outline"
              className="max-w-xs"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder={t('effective.filter')}
              aria-label={t('effective.filter')}
            />
            <label className="flex items-center gap-2 text-sm text-foreground">
              <Switch checked={showAll} onCheckedChange={setShowAll} aria-label={t('effective.showAll')} />
              {t('effective.showAll')}
            </label>
            <span className="ml-auto text-xs text-muted-foreground">
              {t('effective.summary', { held, total: data.permissions.length })}
            </span>
          </div>
          <EffectivePermissionTable data={data} filter={filter} showAll={showAll} />
        </div>
      )}
    </section>
  );
}
