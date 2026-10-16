// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { UseMutationResult } from '@tanstack/react-query';
import { Alert } from '@resources/components/ui/Alert';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Disclosure } from '@resources/components/ui/Disclosure';
import { Spinner } from '@resources/components/ui/Spinner';
import { formatBytes } from '@resources/utils/format';
import type { BundleCategory, BundlePreview } from '@resources/hooks/useSupport';

type SupportBundlePreviewDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  preview: UseMutationResult<BundlePreview, Error, BundleCategory[]>;
};

export function SupportBundlePreviewDialog({ open: isOpen, onOpenChange, preview }: SupportBundlePreviewDialogProps) {
  const { t } = useTranslation('support');
  const data = preview.data;
  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle>{t('bundle.previewTitle')}</DialogTitle>
          <DialogDescription>
            {data
              ? t('bundle.previewDescription', { count: data.files.length, size: formatBytes(data.totalBytes) })
              : t('bundle.categoriesHint')}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-2">
          {preview.isPending && <Spinner />}
          {preview.error && <Alert variant="destructive">{preview.error.message}</Alert>}
          {data?.files.map((f, i) => (
            <Disclosure key={f.name} title={f.name} trailing={formatBytes(f.sizeBytes)} defaultOpen={i === 0}>
              {f.truncated && (
                <Alert variant="info" className="mb-2">
                  {t('bundle.previewTruncated')}
                </Alert>
              )}
              <pre className="max-h-[360px] overflow-auto rounded-md bg-muted p-3 font-mono text-xs break-all whitespace-pre-wrap text-foreground">
                {f.content}
              </pre>
            </Disclosure>
          ))}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
