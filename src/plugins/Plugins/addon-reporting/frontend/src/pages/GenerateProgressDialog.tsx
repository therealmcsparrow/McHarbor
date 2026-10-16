// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
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
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { Spinner } from '@resources/components/ui/Spinner';
import { useGenerateJob } from '../hooks/useReporting';

// What the operator watches while a report is being written.
//
// The bar is driven by the server's own count of sources read, not by a
// timer: collection is the slow phase and reports a source at a time, so
// the movement means something. A bar that animates on a guess is worse
// than a spinner, because it invites the reader to estimate from it.
//
// There is no cancel. Generation is read-only — it collects, computes and
// writes one row — so the only thing a cancel could buy is stopping a few
// loopback reads, at the cost of a half-written library entry to explain.

type Props = {
  /** The job to watch, or null when nothing is running. */
  jobId: string | null;
  /** Called with the report id once the document is stored. */
  onDone: (reportId: string) => void;
  onClose: () => void;
};

export function GenerateProgressDialog({ jobId, onDone, onClose }: Props) {
  const { t, i18n } = useTranslation('reporting');
  const job = useGenerateJob(jobId);

  const state = job.data?.state;
  const reportId = job.data?.report_id;

  // Opening the finished report is the point of the dialog, so it happens
  // as soon as the job settles rather than behind a second click.
  useEffect(() => {
    if (state === 'done' && reportId) onDone(reportId);
  }, [state, reportId, onDone]);

  const failed = state === 'failed' || job.isError;
  const percent = failed ? 100 : (job.data?.percent ?? 0);
  const stage = job.data?.stage ?? 'queued';

  // The backend emits codes as `err.reporting.<name>`; the bundle nests
  // the matching strings under `error.reporting.<name>`. An unknown or
  // missing code — and any code with no translation — falls back to the
  // generic message rather than ever showing the raw code to the user.
  const errorKey = job.data?.code?.replace(/^err\./, 'error.');
  const failureMessage = errorKey && i18n.exists(errorKey, { ns: 'reporting' }) ? t(errorKey) : t('progress.failed');

  return (
    <Dialog
      open={jobId !== null}
      onOpenChange={(nextOpen) => {
        // While it is still running the dialog is not dismissable: the
        // work continues either way, and a dialog that vanishes mid-run
        // leaves the operator with no way back to the result.
        if (!nextOpen && failed) onClose();
      }}
    >
      {jobId ? (
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{t('progress.title')}</DialogTitle>
            <DialogDescription>{t('progress.description')}</DialogDescription>
          </DialogHeader>

          <DialogBody className="space-y-4">
            {failed ? (
              <div className="flex items-start gap-2.5 rounded-lg border border-danger/40 bg-danger/10 p-3">
                <IconAlertTriangle className="mt-0.5 size-4 shrink-0 text-danger" aria-hidden="true" />
                <p className="text-sm text-foreground">{failureMessage}</p>
              </div>
            ) : (
              <>
                <ProgressBar value={percent} size="sm" />
                <div className="flex items-center justify-between gap-3 text-xs">
                  <span className="flex items-center gap-2 text-foreground">
                    <Spinner className="size-3.5" />
                    {t(`progress.stage.${stage}`)}
                  </span>
                  {job.data && job.data.sources_total > 0 ? (
                    <span className="text-muted-foreground">
                      {t('progress.sources', {
                        done: Math.min(job.data.sources_done, job.data.sources_total),
                        total: job.data.sources_total,
                      })}
                    </span>
                  ) : null}
                </div>
              </>
            )}
          </DialogBody>

          <DialogFooter>
            <Button variant="ghost" onClick={onClose} disabled={!failed}>
              {t('action.close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      ) : null}
    </Dialog>
  );
}
