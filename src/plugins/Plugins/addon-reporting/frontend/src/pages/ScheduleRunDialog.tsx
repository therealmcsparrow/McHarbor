// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCheck, IconFileTypeDocx, IconFileTypePdf, IconX } from '@tabler/icons-react';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { cn } from '@resources/utils/cn';
import { useExportScheduleRun, useScheduleRun } from '../hooks/useReporting';
import { DocumentView } from '../blocks/DocumentView';
import { statusPill } from '../blocks/tokens';
import { formatDateTime, runStatusTone } from './ScheduleColumns';
import type { Schedule } from '../types';

// The last run of one schedule.
//
// A dialog rather than a route, because a schedule keeps exactly one run:
// there is no history to navigate and nothing to link to that would still
// mean the same thing tomorrow. The per-target outcome leads, because
// "did it arrive" is the question the page is opened to answer; the
// document itself follows for the reader who wants to check the content.

type Props = {
  schedule: Schedule | null;
  onClose: () => void;
};

/**
 * Backend code to bundle key.
 *
 * The backend emits `err.reporting.<name>`; the bundle nests the matching
 * strings under `error.reporting.<name>`, so only the first segment
 * changes. Same mapping `GenerateProgressDialog` uses — an unknown code
 * falls back to the caller's `defaultValue` rather than echoing a key.
 */
function errorKey(code: string | undefined): string {
  return (code ?? '').replace(/^err\./, 'error.');
}

export function ScheduleRunDialog({ schedule, onClose }: Props) {
  const { t } = useTranslation('reporting');
  const run = useScheduleRun(schedule?.id ?? null);
  const download = useExportScheduleRun();

  return (
    <Dialog open={schedule !== null} onOpenChange={(nextOpen) => !nextOpen && onClose()}>
      {schedule ? (
        <DialogContent className="max-w-4xl">
          <DialogHeader>
            <DialogTitle>{schedule.name}</DialogTitle>
            <DialogDescription>
              {run.data
                ? t('schedules.run.ranAt', {
                    when: formatDateTime(run.data.finished_at || run.data.started_at),
                  })
                : t('schedules.run.loading')}
            </DialogDescription>
          </DialogHeader>

          <DialogBody className="space-y-5">
            {run.isLoading ? (
              <CardSkeleton lines={6} />
            ) : run.isError || !run.data ? (
              <ErrorState
                description={t('schedules.run.error')}
                onRetry={() => {
                  void run.refetch();
                }}
              />
            ) : (
              <>
                <div className="flex flex-wrap items-center gap-2">
                  <span
                    className={cn(
                      'rounded-full px-2.5 py-1 text-xs font-semibold',
                      statusPill[runStatusTone(run.data.status)],
                    )}
                  >
                    {t(`schedules.status.${run.data.status}`)}
                  </span>
                  {run.data.code ? (
                    <span className="text-sm text-danger">
                      {/* eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: backend error codes, and an unknown one is better shown than as a raw key */}
                      {t(errorKey(run.data.code), { defaultValue: run.data.code })}
                    </span>
                  ) : null}
                </div>

                <section>
                  <h3 className="mb-2 text-xs font-semibold tracking-[0.12em] text-muted-foreground uppercase">
                    {t('schedules.run.delivery')}
                  </h3>
                  {run.data.deliveries.length === 0 ? (
                    <p className="text-sm text-muted-foreground">{t('schedules.run.noDelivery')}</p>
                  ) : (
                    <ul className="space-y-1.5">
                      {run.data.deliveries.map((outcome) => (
                        <li
                          key={`${outcome.kind}:${outcome.ref_id}`}
                          className="flex items-center gap-2 text-sm text-foreground"
                        >
                          {outcome.ok ? (
                            <IconCheck className="size-4 shrink-0 text-success" aria-hidden="true" />
                          ) : (
                            <IconX className="size-4 shrink-0 text-danger" aria-hidden="true" />
                          )}
                          <span className="truncate">{outcome.name}</span>
                          <span className="text-xs text-muted-foreground">
                            {outcome.ok
                              ? outcome.attached
                                ? t('schedules.run.attached')
                                : t('schedules.run.linkOnly')
                              : // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: backend error codes fall back to the generic failure label
                                t(errorKey(outcome.code), { defaultValue: t('schedules.run.failed') })}
                          </span>
                        </li>
                      ))}
                    </ul>
                  )}
                </section>

                {run.data.document ? (
                  // The document scrolls inside the dialog body rather than
                  // stretching it: a full report is dozens of screens tall.
                  <section className="max-h-[45vh] overflow-x-hidden overflow-y-auto rounded-lg border border-border">
                    <DocumentView doc={run.data.document} />
                  </section>
                ) : null}
              </>
            )}
          </DialogBody>

          <DialogFooter>
            <Button variant="ghost" onClick={onClose}>
              {t('action.close')}
            </Button>
            <Button
              variant="outline"
              disabled={!run.data?.document || download.isPending}
              onClick={() => download.mutate({ id: schedule.id, format: 'pdf', name: schedule.name })}
            >
              <IconFileTypePdf className="size-4" />
              {t('action.exportPdf')}
            </Button>
            <Button
              variant="outline"
              disabled={!run.data?.document || download.isPending}
              onClick={() => download.mutate({ id: schedule.id, format: 'docx', name: schedule.name })}
            >
              <IconFileTypeDocx className="size-4" />
              {t('action.exportDocx')}
            </Button>
          </DialogFooter>
        </DialogContent>
      ) : null}
    </Dialog>
  );
}
