// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { PERM } from '@core/auth/permissionKeys';
import { useMergeEnvironments, type MergeResult } from '../hooks/useEnvironmentMigration';

type MergeEnvironmentsDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  target: { id: string; name: string };
  candidates: { id: string; name: string; connectionType: string }[];
  defaultSourceId?: string;
};

export function MergeEnvironmentsDialog({
  open: isOpen,
  onOpenChange,
  target,
  candidates,
  defaultSourceId,
}: MergeEnvironmentsDialogProps) {
  const { t } = useTranslation('environments');
  const merge = useMergeEnvironments();

  const selectable = useMemo(() => candidates.filter((entry) => entry.id !== target.id), [candidates, target.id]);
  const [sourceId, setSourceId] = useState(defaultSourceId ?? selectable[0]?.id ?? '');
  const [plan, setPlan] = useState<MergeResult | null>(null);

  useEffect(() => {
    if (!isOpen) {
      setPlan(null);
      return;
    }
    if (!sourceId) return;
    merge.mutate({ targetId: target.id, sourceId, dryRun: true }, { onSuccess: (result) => setPlan(result) });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, sourceId, target.id]);

  const apply = () => {
    if (!sourceId) return;
    merge.mutate({ targetId: target.id, sourceId }, { onSuccess: () => onOpenChange(false) });
  };

  const moved = plan?.moved ?? [];

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('merge.title')}</DialogTitle>
          <DialogDescription>{t('merge.description', { target: target.name })}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div>
            <Label className="mb-2">{t('merge.source')}</Label>
            <Select
              value={sourceId}
              ariaLabel={t('merge.source')}
              options={selectable.map((entry) => ({
                value: entry.id,
                label: `${entry.name} (${t(`connectionTypes.${entry.connectionType}`)})`,
              }))}
              onChange={(value) => {
                setSourceId(value);
                setPlan(null);
              }}
            />
            <p className="mt-1 text-xs text-muted-foreground">{t('merge.sourceHint')}</p>
          </div>

          <div className="flex items-start gap-2 rounded-md bg-warning/10 p-3">
            <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
            <p className="text-sm text-foreground">{t('merge.warning')}</p>
          </div>

          {merge.isPending && !plan ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Spinner className="h-4 w-4" />
              {t('merge.planning')}
            </div>
          ) : plan ? (
            <div>
              <h4 className="mb-2 text-sm font-medium text-foreground">
                {t('merge.planTitle', { count: plan.totalRows })}
              </h4>
              {moved.length === 0 ? (
                <p className="text-sm text-muted-foreground">{t('merge.planEmpty')}</p>
              ) : (
                <div className="max-h-64 overflow-y-auto rounded-lg border border-border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('merge.columns.table')}</TableHead>
                        <TableHead className="w-24 text-right">{t('merge.columns.rows')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {moved.map((entry) => (
                        <TableRow key={`${entry.table}.${entry.column}`}>
                          <TableCell className="font-mono text-xs">{entry.table}</TableCell>
                          <TableCell className="text-right">{entry.rows}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
              {plan.defaultMoved && <p className="mt-2 text-xs text-muted-foreground">{t('merge.defaultMoved')}</p>}
            </div>
          ) : null}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            permission={PERM.ENVIRONMENTS_MANAGE}
            variant="destructive"
            onClick={apply}
            disabled={!sourceId || !plan || merge.isPending}
          >
            {merge.isPending && <Spinner className="mr-2 h-4 w-4" />}
            {t('merge.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
