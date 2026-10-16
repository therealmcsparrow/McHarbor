// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconStethoscope, IconTool } from '@tabler/icons-react';
import { toast } from 'sonner';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { StatusError } from '@resources/components/ui/StatusError';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { PERM } from '@core/auth/permissionKeys';
import { ACTIVE_CHECK_PHASES } from '../types';
import { useCancelCheck, useCheckStatus, useRepair, useStartCheck } from '../hooks/useDatabaseCheck';
import { DatabaseCheckDialog } from './DatabaseCheckDialog';
import { DatabaseFindingsTable } from './DatabaseFindingsTable';

export function DatabaseHealthCard() {
  const { t } = useTranslation('database');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const currentStatus = useCheckStatus(true);
  const start = useStartCheck();
  const cancel = useCancelCheck();
  const repair = useRepair();

  const data = currentStatus.data ?? null;
  const running = data ? ACTIVE_CHECK_PHASES.includes(data.phase) : false;
  const findings = data?.findings ?? [];
  const destructiveSelected = findings.some((f) => selected.has(f.id) && f.destructive);

  const toggle = (id: string) => {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const runCheck = () => {
    setSelected(new Set());
    setDialogOpen(true);
    start.mutate(undefined, {
      onError: (_error) => {
        setDialogOpen(false);
      },
    });
  };

  const applyRepairs = () => {
    repair.mutate(Array.from(selected), {
      onSuccess: (result) => {
        setConfirmOpen(false);
        setSelected(new Set());
        toast.success(t('check.toast.repaired', { count: result.rows }));
      },
      onError: (_error) => {
        setConfirmOpen(false);
      },
    });
  };

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>{t('check.title')}</CardTitle>
          <TypographyText size="sm" tone="muted">
            {t('check.description')}
          </TypographyText>
        </CardHeader>
        <CardContent>
          <Stack gap={4}>
            <Flex align="center" justify="between" gap={3} wrap>
              {data && (data.phase === 'failed' || data.phase === 'cancelled') ? (
                <Flex align="center" gap={2}>
                  <TypographyText size="sm" tone="muted">
                    {t(`check.phase.${data.phase}`)}
                  </TypographyText>
                  <StatusError error={data.errorDetail} />
                </Flex>
              ) : (
                <TypographyText size="sm" tone="muted">
                  {data
                    ? t('check.lastRun', {
                        failed: data.counts.fail ?? 0,
                        warned: data.counts.warn ?? 0,
                      })
                    : t('check.never')}
                </TypographyText>
              )}
              <Flex align="center" gap={2}>
                <Button
                  permission={PERM.DATABASE_MANAGE}
                  variant="outline"
                  disabled={running || start.isPending}
                  onClick={runCheck}
                >
                  <IconStethoscope className="size-4" aria-hidden />
                  {running ? t('check.running') : t('check.run')}
                </Button>
                <Button
                  permission={PERM.DATABASE_MANAGE}
                  disabled={selected.size === 0 || repair.isPending || running}
                  onClick={() => setConfirmOpen(true)}
                >
                  <IconTool className="size-4" aria-hidden />
                  {t('check.repair', { count: selected.size })}
                </Button>
              </Flex>
            </Flex>

            {findings.length > 0 && <DatabaseFindingsTable findings={findings} selected={selected} onToggle={toggle} />}
          </Stack>
        </CardContent>
      </Card>

      <DatabaseCheckDialog
        open={dialogOpen}
        status={data}
        cancelling={cancel.isPending}
        onCancel={() => cancel.mutate()}
        onClose={() => setDialogOpen(false)}
      />

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('check.confirmTitle')}
        description={
          destructiveSelected
            ? t('check.confirmDestructive', { count: selected.size })
            : t('check.confirmBody', { count: selected.size })
        }
        confirmLabel={t('check.repairConfirm')}
        variant={destructiveSelected ? 'destructive' : 'default'}
        loading={repair.isPending}
        onConfirm={applyRepairs}
      />
    </>
  );
}
