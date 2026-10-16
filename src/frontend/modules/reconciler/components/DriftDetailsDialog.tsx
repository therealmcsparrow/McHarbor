// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@resources/components/ui/Dialog';
import { timeAgo } from '@resources/utils/format';
import type { DesiredState } from '../hooks/useReconciler';

type DriftDetailsDialogProps = {
  state: DesiredState | null;
  onOpenChange: (open: boolean) => void;
};

export function DriftDetailsDialog({ state, onOpenChange }: DriftDetailsDialogProps) {
  const { t } = useTranslation('reconciler');
  const report = state?.lastReport;
  const diffs = report?.diffs ?? [];

  return (
    <Dialog open={state !== null} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('details.title', { name: state?.name ?? '' })}</DialogTitle>
          <DialogDescription>
            {t('details.description', { time: report ? timeAgo(report.checkedAt) : '—' })}
          </DialogDescription>
        </DialogHeader>
        {diffs.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('details.none')}</p>
        ) : (
          <div className="max-h-[60vh] overflow-auto">
            <table className="w-full text-left text-sm">
              <thead className="text-xs text-muted-foreground">
                <tr>
                  <th className="py-1 pr-3 font-medium">{t('details.field')}</th>
                  <th className="py-1 pr-3 font-medium">{t('details.expected')}</th>
                  <th className="py-1 pr-3 font-medium">{t('details.actual')}</th>
                  <th className="py-1 font-medium">{t('details.firstSeen')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {diffs.map((diff) => (
                  <tr key={`${diff.class}|${diff.field}`} className="align-top">
                    <td className="py-1.5 pr-3">
                      <Badge variant="warning" className="mr-2">
                        {t(`class.${diff.class}`)}
                      </Badge>
                      <span className="font-mono text-xs break-all">{diff.field}</span>
                    </td>
                    <td className="py-1.5 pr-3 font-mono text-xs break-all">{diff.expected}</td>
                    <td className="py-1.5 pr-3 font-mono text-xs break-all">{diff.actual}</td>
                    <td className="py-1.5 text-xs text-muted-foreground">
                      {diff.firstSeenAt ? timeAgo(diff.firstSeenAt) : '—'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
