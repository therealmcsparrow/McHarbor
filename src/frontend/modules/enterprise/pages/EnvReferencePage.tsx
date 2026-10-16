// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { ManualCopyDialog } from '@resources/components/ManualCopyDialog';
import { Alert } from '@resources/components/ui/Alert';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { Table, TableBody, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { PageHeader } from '@resources/layout/PageHeader';
import { copyToClipboard } from '@resources/utils/clipboard';
import { EnvReferenceRow } from '../components/EnvReferenceRow';
import { envVariables, filterEnvVariables, groupBySection, type EnvFilter } from '../utils/envReference';

const ALL_VARIABLES = envVariables();

export default function EnvReferencePage() {
  const { t } = useTranslation('envref');
  const canView = usePermission(PERM.SETTINGS_VIEW);
  const [filter, setFilter] = useState<EnvFilter>({ query: '', scope: 'all', sensitiveOnly: false });
  const [copied, setCopied] = useState<string | null>(null);
  const [manual, setManual] = useState<string | null>(null);

  const visible = useMemo(() => filterEnvVariables(ALL_VARIABLES, filter), [filter]);
  const groups = useMemo(() => groupBySection(visible), [visible]);

  const copy = async (nameValue: string) => {
    const result = await copyToClipboard(nameValue);
    if (!result.ok) {
      setManual(nameValue);
      return;
    }
    setCopied(nameValue);
    toast.success(t('copied'));
    window.setTimeout(() => setCopied((cur) => (cur === nameValue ? null : cur)), 2000);
  };

  if (!canView) {
    return (
      <div className="space-y-6">
        <PageHeader title={t('title')} description={t('description')} headingLeaf />
        <Alert variant="warning">{t('permission.requires', { ns: 'common', permission: PERM.SETTINGS_VIEW })}</Alert>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader title={t('title')} description={t('description')} headingLeaf />
      <div className="flex flex-wrap items-end gap-4">
        <div className="min-w-[240px] flex-1">
          <Input
            type="search"
            value={filter.query}
            onChange={(e) => setFilter((f) => ({ ...f, query: e.target.value }))}
            placeholder={t('searchPlaceholder')}
            aria-label={t('searchLabel')}
          />
        </div>
        <Select
          value={filter.scope}
          onChange={(v) => setFilter((f) => ({ ...f, scope: v === 'server' || v === 'agent' ? v : 'all' }))}
          ariaLabel={t('filters.scope')}
          searchable={false}
          className="w-40"
          options={[
            { value: 'all', label: t('filters.allScopes') },
            { value: 'server', label: t('scope.server') },
            { value: 'agent', label: t('scope.agent') },
          ]}
        />
        <div className="flex items-center gap-2">
          <Switch
            id="envref-sensitive"
            checked={filter.sensitiveOnly}
            onCheckedChange={(v) => setFilter((f) => ({ ...f, sensitiveOnly: v }))}
          />
          <Label htmlFor="envref-sensitive">{t('filters.sensitiveOnly')}</Label>
        </div>
        <TypographyText tone="muted" size="sm">
          {t('count', { count: visible.length })}
        </TypographyText>
      </div>
      {groups.length === 0 ? (
        <EmptyState title={t('empty')} />
      ) : (
        groups.map((group) => (
          <Card key={group.section}>
            <CardHeader>
              <CardTitle>{t(`sections.${group.section}`)}</CardTitle>
            </CardHeader>
            <CardContent className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[28%]">{t('columns.name')}</TableHead>
                    <TableHead className="w-[16%]">{t('columns.default')}</TableHead>
                    <TableHead>{t('columns.description')}</TableHead>
                    <TableHead className="w-[18%]">{t('columns.flags')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {group.items.map((v) => (
                    <EnvReferenceRow
                      key={`${v.scope}:${v.name}`}
                      variable={v}
                      copied={copied === v.name}
                      onCopy={(itemName) => void copy(itemName)}
                    />
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        ))
      )}
      <ManualCopyDialog
        open={manual !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) setManual(null);
        }}
        value={manual ?? ''}
        title={t('manualCopy')}
      />
    </div>
  );
}
