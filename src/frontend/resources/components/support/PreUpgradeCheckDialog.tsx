// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconExternalLink, IconRefresh } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { type PluginCompat, type PreUpgradeReport, usePreUpgradeCheck } from '@resources/hooks/useSupport';
import { CheckResultRow } from './CheckResultRow';

const compatibilityMatrixUrl =
  'https://github.com/therealmcsparrow/mcharbor/blob/main/.documentation/getting-started/deployment/compatibility.md';

type PreUpgradeCheckDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  target: string;
};

const compatTone: Record<PluginCompat['compatible'], 'success' | 'destructive' | 'warning'> = {
  yes: 'success',
  no: 'destructive',
  unknown: 'warning',
};

export function PreUpgradeCheckDialog({ open: isOpen, onOpenChange, target }: PreUpgradeCheckDialogProps) {
  const { t } = useTranslation('support');
  const report = usePreUpgradeCheck(target, isOpen);
  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('preUpgrade.title')}</DialogTitle>
          <DialogDescription>
            {t('preUpgrade.description', { current: report.data?.currentVersion ?? '-', target })}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {report.isLoading && <Spinner />}
          {report.error && <Alert variant="destructive">{report.error.message}</Alert>}
          {report.data && <ReportView report={report.data} />}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => void report.refetch()} disabled={report.isFetching}>
            {report.isFetching ? <Spinner size="sm" /> : <IconRefresh className="size-4" />}
            {t('preUpgrade.rerun')}
          </Button>
          <Button onClick={() => onOpenChange(false)}>{t('actions.close', { ns: 'common' })}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ReportView({ report }: { report: PreUpgradeReport }) {
  const { t } = useTranslation('support');
  const isExternal = report.databaseDriver !== 'sqlite';
  return (
    <>
      <ul>
        {report.checks.map((c) => (
          <CheckResultRow key={c.id} id={c.id} result={c} />
        ))}
      </ul>
      {report.plugins.length > 0 && (
        <section className="space-y-2">
          <h3 className="text-sm font-semibold text-foreground">{t('preUpgrade.pluginsTitle')}</h3>
          <ul className="space-y-1 text-sm">
            {report.plugins.map((p) => (
              <li
                key={p.slug}
                className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border px-3 py-2"
              >
                <span className="font-mono text-foreground">
                  {p.slug}@{p.version}
                </span>
                <span className="flex items-center gap-2 text-xs text-muted-foreground">
                  {p.range ? t('preUpgrade.pluginRange', { range: p.range }) : t('preUpgrade.noRange')}
                  <Badge variant={compatTone[p.compatible]}>{t(`preUpgrade.compat.${p.compatible}`)}</Badge>
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}
      <section className="space-y-2" aria-labelledby="pre-upgrade-compat">
        <h3 id="pre-upgrade-compat" className="text-sm font-semibold text-foreground">
          {t('preUpgrade.compatTitle')}
        </h3>
        <a
          className="inline-flex items-center gap-1 text-sm text-primary underline"
          href={compatibilityMatrixUrl}
          target="_blank"
          rel="noreferrer"
        >
          <IconExternalLink className="size-4" aria-hidden="true" />
          {t('preUpgrade.compatLink')}
        </a>
      </section>
      <section className="space-y-2" aria-labelledby="pre-upgrade-rollback">
        <h3 id="pre-upgrade-rollback" className="text-sm font-semibold text-foreground">
          {t('preUpgrade.rollbackTitle')}
        </h3>
        <ul className="list-disc space-y-1 pl-5 text-sm text-muted-foreground">
          <li>{t(isExternal ? 'preUpgrade.rollbackExternal' : 'preUpgrade.rollbackSqlite')}</li>
          <li>{t('preUpgrade.rollbackImage')}</li>
          <li>{t('preUpgrade.rollbackKey')}</li>
        </ul>
      </section>
    </>
  );
}
