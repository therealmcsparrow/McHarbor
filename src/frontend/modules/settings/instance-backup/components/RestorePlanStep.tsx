// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCheck, IconMinus } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { formatDate } from '@resources/utils/format';
import type { RestorePlan } from '../hooks/useInstanceRestore';

export const CONFIRM_WORD = 'RESTORE';

type RestorePlanStepProps = {
  plan: RestorePlan;
  confirmation: string;
  onConfirmationChange: (value: string) => void;
};

function sum(values: number[]): number {
  return values.reduce((a, b) => a + b, 0);
}

export function RestorePlanStep({ plan, confirmation, onConfirmationChange }: RestorePlanStepProps) {
  const { t } = useTranslation('instancebackup');
  const m = plan.manifest;

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-muted-foreground">
        {t('plan.from', {
          host: m.hostname || '-',
          version: m.version,
          backend: m.backend,
          date: formatDate(m.createdAt),
          user: m.createdBy || '-',
        })}
      </p>
      {plan.refusals.map((r) => (
        <Alert key={r} variant="destructive">
          {t(`plan.refusals.${r}`)}
        </Alert>
      ))}
      {plan.warnings.map((w) => (
        <Alert key={w} variant="warning">
          {t(`plan.warnings.${w}`)}
        </Alert>
      ))}
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('plan.category')}</TableHead>
            <TableHead>{t('plan.restored')}</TableHead>
            <TableHead className="text-right">{t('plan.now')}</TableHead>
            <TableHead className="text-right">{t('plan.fromExport')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {plan.categories.map((c) => (
            <TableRow key={c.name}>
              <TableCell>{t(`categories.${c.name}`)}</TableCell>
              <TableCell>
                {c.selected ? (
                  <IconCheck className="size-4 text-success" aria-label={t('plan.yes')} />
                ) : (
                  <IconMinus className="size-4 text-muted-foreground" aria-label={t('plan.no')} />
                )}
              </TableCell>
              <TableCell className="text-right">{sum(c.tables.map((x) => x.targetRows))}</TableCell>
              <TableCell className="text-right">{sum(c.tables.map((x) => x.artifactRows))}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <ul className="list-disc pl-5 text-sm text-muted-foreground">
        {plan.restoresKeys && <li>{t('plan.restoresKeys')}</li>}
        {plan.restoresSecret && <li>{t('plan.restoresSecret')}</li>}
        {plan.restoresFiles && <li>{t('plan.restoresFiles')}</li>}
        {plan.plugins.length > 0 && <li>{t('plan.plugins', { count: plan.plugins.length })}</li>}
        <li>{t('plan.restart')}</li>
      </ul>
      {plan.refusals.length === 0 && (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="ib-confirm-restore">{t('plan.confirm', { word: CONFIRM_WORD })}</Label>
          <Input
            id="ib-confirm-restore"
            autoComplete="off"
            value={confirmation}
            onChange={(e) => onConfirmationChange(e.target.value)}
          />
        </div>
      )}
    </div>
  );
}
