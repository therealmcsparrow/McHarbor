// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// What used to be the inline panel an accordion row expanded into. Same fields,
// same attribute dump — shown in a modal so every row keeps one height.

import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { InfoRow } from '@resources/components/ui/InfoRow';
import type { LifecycleEvent } from '../hooks/useLifecycle';
import { humanizeAction, parseMetadata } from './lifecycle-format';

type LifecycleDetailsDialogProps = {
  event: LifecycleEvent | null;
  onOpenChange: (open: boolean) => void;
};

const DASH = '—';

export function LifecycleDetailsDialog({ event: evt, onOpenChange }: LifecycleDetailsDialogProps) {
  const { t } = useTranslation('settings');
  const meta = evt ? parseMetadata(evt.metadata) : null;

  return (
    <Dialog open={!!evt} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('lifecycle.detailsTitle')}</DialogTitle>
          <DialogDescription>
            {evt ? `${humanizeAction(evt.action, t)} — ${evt.subjectName ?? evt.subjectId}` : ''}
          </DialogDescription>
        </DialogHeader>
        {evt && (
          <DialogBody className="space-y-4">
            <div>
              <InfoRow label={t('lifecycle.detailId')}>
                <span className="font-mono text-xs">{evt.id}</span>
              </InfoRow>
              <InfoRow label={t('lifecycle.detailSubject')}>
                <span className="font-mono text-xs">
                  {evt.subjectType} / {evt.subjectId}
                </span>
              </InfoRow>
              <InfoRow label={t('lifecycle.detailEvent')}>
                <span className="font-mono text-xs">
                  {evt.eventType} / {evt.action}
                </span>
              </InfoRow>
              <InfoRow label={t('lifecycle.detailState')}>
                <span className="font-mono text-xs">{evt.state ?? DASH}</span>
              </InfoRow>
            </div>

            {meta && Object.keys(meta).length > 0 && (
              <div>
                <div className="mb-1 text-xs text-muted-foreground">{t('lifecycle.detailAttributes')}</div>
                <pre className="max-h-64 overflow-auto rounded-md border border-border bg-muted/30 p-3 font-mono text-[11px] leading-relaxed text-foreground">
                  {Object.entries(meta)
                    .map(([k, v]) => `${k}=${v}`)
                    .join('\n')}
                </pre>
              </div>
            )}
          </DialogBody>
        )}
      </DialogContent>
    </Dialog>
  );
}
