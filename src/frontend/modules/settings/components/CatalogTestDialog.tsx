// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { IconRefresh } from '@tabler/icons-react';
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
import { PERM } from '@core/auth/permissionKeys';
import type { Catalog } from '../hooks/useCatalogs';
import { CATALOG_TEST_STEPS, useCatalogTest, type CatalogTestStatus } from '../hooks/useCatalogTest';
import { CatalogTestStepCard } from './CatalogTestStepCard';

const VERDICT_BADGE: Record<CatalogTestStatus, 'success' | 'warning' | 'destructive' | 'secondary'> = {
  pending: 'secondary',
  running: 'secondary',
  ok: 'success',
  warn: 'warning',
  fail: 'destructive',
  skipped: 'secondary',
};

interface CatalogTestDialogProps {
  catalog: Catalog | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Runs the catalog probe and shows it stepping through its five checks.
 *
 * All five rows render up front in `pending` state so the plan is visible
 * before the first result lands — a modal that grows a row at a time
 * tells you nothing about how much is still to come.
 */
export function CatalogTestDialog({ catalog, open: isOpen, onOpenChange }: CatalogTestDialogProps) {
  const { t } = useTranslation('settings');
  const { state, start, reset } = useCatalogTest();
  const catalogId = catalog?.id ?? null;

  useEffect(() => {
    if (isOpen && catalogId) {
      void start(catalogId);
    }
    if (!isOpen) {
      reset();
    }
  }, [isOpen, catalogId, start, reset]);

  const verdict: CatalogTestStatus = state.running
    ? 'running'
    : (state.result?.status ?? (state.errorCode ? 'fail' : 'pending'));

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('catalogs.test.title', { name: catalog?.name ?? '' })}</DialogTitle>
          <DialogDescription>{t('catalogs.test.description')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-3">
          <div className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/20 p-3">
            <div className="flex items-center gap-2">
              <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                {t('catalogs.test.overallStatus')}
              </span>
              <Badge variant={VERDICT_BADGE[verdict]}>{t(`catalogs.test.verdict.${verdict}`)}</Badge>
            </div>
            {state.result && verdict !== 'fail' ? (
              <span className="shrink-0 text-xs text-muted-foreground">
                {t('catalogs.test.verdictEntries', {
                  total: state.result.entries,
                  kind: t(`catalogs.kind.${state.result.kind}`),
                })}
              </span>
            ) : null}
          </div>

          {catalog ? (
            <code className="block truncate rounded-lg border border-border bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
              {catalog.url}
            </code>
          ) : null}

          <ol className="space-y-2">
            {CATALOG_TEST_STEPS.map((key) => (
              <CatalogTestStepCard key={key} stepKey={key} step={state.steps[key]} />
            ))}
          </ol>

          {state.errorCode ? (
            <p className="text-sm text-destructive">
              {t(`catalogs.test.error.${state.errorCode}`, {
                // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: catalog test error codes come from the backend and fall back to a generic label
                defaultValue: t('catalogs.test.error.err.unknown'),
              })}
            </p>
          ) : null}
        </DialogBody>

        <DialogFooter>
          <Button
            permission={PERM.SETTINGS_MANAGE}
            variant="outline"
            onClick={() => catalogId && void start(catalogId)}
            disabled={state.running || !catalogId}
          >
            <IconRefresh className="size-4" />
            {t('catalogs.test.retry')}
          </Button>
          <Button onClick={() => onOpenChange(false)}>{t('actions.close', { ns: 'common' })}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
