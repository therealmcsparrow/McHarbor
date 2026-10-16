// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCopy, IconPlayerPlay, IconRefresh } from '@tabler/icons-react';
import { toast } from 'sonner';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Spinner } from '@resources/components/ui/Spinner';
import { ManualCopyDialog } from '@resources/components/ManualCopyDialog';
import { PageHeader } from '@resources/layout/PageHeader';
import { copyToClipboard } from '@resources/utils/clipboard';
import { formatDate } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { CheckResultRow } from '@resources/components/support/CheckResultRow';
import { type DiagnosticsReport, useDiagnostics, useRunCheck } from '@resources/hooks/useSupport';

export default function DiagnosticsPage() {
  const { t } = useTranslation('support');
  const canView = usePermission(PERM.SUPPORT_VIEW);
  const report = useDiagnostics(canView);

  return (
    <div className="space-y-6">
      <PageHeader title={t('diagnostics.title')} description={t('diagnostics.description')} headingLeaf />
      {!canView ? (
        <Alert variant="warning">{t('permission.requires', { ns: 'common', permission: PERM.SUPPORT_VIEW })}</Alert>
      ) : report.isLoading ? (
        <Spinner />
      ) : report.error ? (
        <Alert variant="destructive">{report.error.message}</Alert>
      ) : report.data ? (
        <DiagnosticsView report={report.data} refreshing={report.isFetching} onRunAll={() => void report.refetch()} />
      ) : null}
    </div>
  );
}

type DiagnosticsViewProps = { report: DiagnosticsReport; refreshing: boolean; onRunAll: () => void };

function DiagnosticsView({ report, refreshing, onRunAll }: DiagnosticsViewProps) {
  const { t } = useTranslation('support');
  const runCheck = useRunCheck();
  const [manual, setManual] = useState(false);
  const json = JSON.stringify(report, null, 2);
  const count = (s: string) => report.checks.filter((c) => c.status === s).length;

  const copy = async () => {
    const result = await copyToClipboard(json);
    if (result.ok) toast.success(t('clipboard.copied', { ns: 'common' }));
    else setManual(true);
  };

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-3">
        <div className="space-y-1">
          <CardTitle>
            {t('diagnostics.summary', { pass: count('pass'), warn: count('warn'), fail: count('fail') })}
          </CardTitle>
          <p className="text-xs text-muted-foreground">
            {t('diagnostics.generatedAt', { time: formatDate(report.generatedAt) })}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => void copy()}>
            <IconCopy className="size-4" />
            {t('diagnostics.copyJson')}
          </Button>
          <Button onClick={onRunAll} disabled={refreshing} permission={PERM.SUPPORT_VIEW}>
            {refreshing ? <Spinner size="sm" /> : <IconRefresh className="size-4" />}
            {t('diagnostics.runAll')}
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {runCheck.error && (
          <Alert variant="destructive" className="mb-3">
            {runCheck.error.message}
          </Alert>
        )}
        <ul>
          {report.checks.map((c) => (
            <CheckResultRow
              key={c.id}
              id={c.id}
              result={c}
              action={
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t('diagnostics.runCheck', { check: t(`check.${c.id}`) })}
                  disabled={runCheck.isPending && runCheck.variables === c.id}
                  onClick={() => runCheck.mutate(c.id)}
                >
                  {runCheck.isPending && runCheck.variables === c.id ? (
                    <Spinner size="sm" />
                  ) : (
                    <IconPlayerPlay className="size-4" />
                  )}
                </Button>
              }
            />
          ))}
        </ul>
      </CardContent>
      <ManualCopyDialog open={manual} onOpenChange={setManual} value={json} />
    </Card>
  );
}
