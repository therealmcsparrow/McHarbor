// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCheck, IconCopy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { useFlash } from '@resources/hooks/useFlash';
import { useErrorDetails } from '@resources/stores/errorDetails';
import type { ErrorDetails } from '@resources/utils/api-mutation';
import { copyToClipboard } from '@resources/utils/clipboard';
import { formatDate } from '@resources/utils/format';

type Row = { label: string; value: string; mono?: boolean };
type Labels = Record<'operation' | 'summary' | 'code' | 'status' | 'requestId' | 'time', string>;

function rowsOf(details: ErrorDetails, label: Labels): Row[] {
  const rows: Row[] = [];
  if (details.operation) rows.push({ label: label.operation, value: details.operation });
  rows.push({ label: label.summary, value: details.summary });
  if (details.code) rows.push({ label: label.code, value: details.code, mono: true });
  if (details.status) rows.push({ label: label.status, value: String(details.status), mono: true });
  if (details.requestId) rows.push({ label: label.requestId, value: details.requestId, mono: true });
  rows.push({ label: label.time, value: formatDate(details.at) });
  return rows;
}

/** The one dialog every error toast's "Details" action opens; mounted beside the toaster. */
export function ErrorDetailsDialog() {
  const { t } = useTranslation('common');
  const details = useErrorDetails((s) => s.open);
  const closeDetails = useErrorDetails((s) => s.close);
  const { flashing: copied, flash } = useFlash();

  if (!details) return null;
  const rows = rowsOf(details, {
    operation: t('errorDetails.operation'),
    summary: t('errorDetails.summary'),
    code: t('errorDetails.code'),
    status: t('errorDetails.status'),
    requestId: t('errorDetails.requestId'),
    time: t('errorDetails.time'),
  });
  const transcript = [
    ...rows.map((row) => `${row.label}: ${row.value}`),
    ...(details.detail ? [`${t('errorDetails.cause')}:`, details.detail] : []),
  ].join('\n');

  function handleCopy() {
    void copyToClipboard(transcript).then((result) => {
      if (result.ok) flash();
    });
  }

  return (
    <Dialog open onOpenChange={(next) => !next && closeDetails()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('statusError.title')}</DialogTitle>
          <DialogDescription>{t('errorDetails.description')}</DialogDescription>
        </DialogHeader>
        <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1.5 text-sm">
          {rows.map((row) => (
            <div key={row.label} className="contents">
              <dt className="text-muted-foreground">{row.label}</dt>
              <dd className={row.mono ? 'font-mono text-xs leading-5 break-all' : 'break-words'}>{row.value}</dd>
            </div>
          ))}
        </dl>
        {details.detail && (
          <div className="space-y-1">
            <p className="text-sm text-muted-foreground">{t('errorDetails.cause')}</p>
            <pre className="max-h-[45vh] overflow-auto rounded-md border border-border bg-muted p-3 text-xs break-words whitespace-pre-wrap text-foreground">
              {details.detail}
            </pre>
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={handleCopy}>
            {copied ? (
              <IconCheck className="mr-1 size-3.5" aria-hidden="true" />
            ) : (
              <IconCopy className="mr-1 size-3.5" aria-hidden="true" />
            )}
            {copied ? t('clipboard.copied') : t('errorDetails.copyAll')}
          </Button>
          <Button onClick={closeDetails}>{t('actions.close')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
