// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { IconTrash } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { PERM } from '@core/auth/permissionKeys';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { StatusError } from '@resources/components/ui/StatusError';
import { DestroyPreviewDialog } from './DestroyPreviewDialog';
import type { GitOpsPRPreview, PreviewStatus } from '../types';

type PreviewsPanelProps = {
  previews: GitOpsPRPreview[];
};

function statusVariant(statusValue: PreviewStatus) {
  switch (statusValue) {
    case 'active':
      return 'success' as const;
    case 'demolishing':
      return 'warning' as const;
    case 'failed':
    case 'refused':
      return 'destructive' as const;
    default:
      return 'secondary' as const;
  }
}

function remaining(expiresAt: string) {
  if (!expiresAt) return '';
  const ms = new Date(expiresAt).getTime() - Date.now();
  if (Number.isNaN(ms)) return '';
  if (ms <= 0) return '0h';
  const hours = Math.floor(ms / 3_600_000);
  if (hours >= 24) return `${Math.floor(hours / 24)}d ${hours % 24}h`;
  if (hours >= 1) return `${hours}h`;
  return `${Math.max(1, Math.floor(ms / 60_000))}m`;
}

export function PreviewsPanel({ previews }: PreviewsPanelProps) {
  const { t } = useTranslation('gitops');
  const [target, setTarget] = useState<GitOpsPRPreview | null>(null);

  if (previews.length === 0) return null;

  return (
    <div>
      <h3 className="mb-2 text-sm font-semibold text-foreground">{t('previews')}</h3>
      <div className="space-y-2">
        {previews.map((preview) => (
          <div key={preview.id} className="mc-glass rounded-md px-3 py-2 text-sm">
            <div className="flex items-center justify-between gap-2">
              <span className="min-w-0 truncate font-medium">
                {t('previewRow.heading', { pr: preview.prNumber, title: preview.prTitle })}
              </span>
              <div className="flex shrink-0 items-center gap-2">
                <Badge variant={statusVariant(preview.status)}>{t(`previewStatus.${preview.status}`)}</Badge>
                <StatusError error={preview.lastError} />
                <Button
                  permission={PERM.GITOPS_DELETE}
                  variant="ghost"
                  size="icon"
                  aria-label={t('destroyPreview.action')}
                  onClick={() => setTarget(preview)}
                >
                  <IconTrash className="size-4" />
                </Button>
              </div>
            </div>

            <div className="text-xs text-muted-foreground">
              {preview.sourceBranch} → {preview.targetBranch} · {preview.commitSha}
            </div>

            <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
              {preview.url ? (
                <a
                  href={preview.url}
                  target="_blank"
                  rel="noreferrer"
                  className="text-primary underline underline-offset-2"
                >
                  {preview.url}
                </a>
              ) : (
                <span>{t('previewRow.noUrl')}</span>
              )}
              {preview.expiresAt && (
                <span>
                  {t('previewRow.expires', {
                    at: preview.expiresAt,
                    remaining: remaining(preview.expiresAt),
                  })}
                </span>
              )}
              {preview.tornDownBy && <span>{t('previewRow.tornDownBy', { reason: preview.tornDownBy })}</span>}
              {preview.commentError && <span>{t('previewRow.commentFailed')}</span>}
            </div>
          </div>
        ))}
      </div>

      <DestroyPreviewDialog
        open={target !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) setTarget(null);
        }}
        preview={target}
      />
    </div>
  );
}
