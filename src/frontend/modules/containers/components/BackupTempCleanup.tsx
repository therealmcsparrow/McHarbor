// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEraser } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { TooltipFocusTarget } from '@resources/components/ui/TooltipFocusTarget';
import { errorDetailsOf } from '@resources/utils/api-mutation';
import { formatBytes } from '@resources/utils/format';
import { useBackupTempCleanup, type BackupTempCleanupResult, type BackupTempStatus } from '../hooks/useBackupTemp';

type Translate = (key: string, options?: Record<string, unknown>) => string;

/** Counts come from the server; a non-finite or negative value is treated as zero before it is announced. */
const count = (value: number) => (Number.isFinite(value) && value > 0 ? value : 0);

function resultText(result: BackupTempCleanupResult, t: Translate) {
  const removed = count(result.removedEntries);
  const keptOwned = count(result.keptOwned);
  const keptRecent = count(result.keptRecent);
  const parts: string[] = [
    removed > 0
      ? t('backupTemp.cleanup.result', { count: removed, size: formatBytes(count(result.removedBytes)) })
      : t('backupTemp.cleanup.resultNothing'),
  ];
  if (keptOwned > 0) parts.push(t('backupTemp.cleanup.keptOwned', { count: keptOwned }));
  if (keptRecent > 0) parts.push(t('backupTemp.cleanup.keptRecent', { count: keptRecent }));
  return parts.join(' ');
}

/** Manual "Clean up temp": confirm, run once, then announce what was removed and what was kept and why. */
export function BackupTempCleanup({ temp }: { temp: BackupTempStatus | undefined }) {
  const { t } = useTranslation('containers');
  const cleanup = useBackupTempCleanup();
  const [isOpen, setOpen] = useState(false);
  const [result, setResult] = useState<BackupTempCleanupResult | null>(null);
  const outputRef = useRef<HTMLOutputElement>(null);
  const nothing = !temp || temp.entries === 0;

  // ConfirmDialog returns focus to the trigger on the next frame, and the trigger is disabled once the temp
  // directory is empty; focus the result two frames later so it lands there and not on a dead control.
  useEffect(() => {
    if (!result) return;
    let inner = 0;
    const outer = requestAnimationFrame(() => {
      inner = requestAnimationFrame(() => outputRef.current?.focus());
    });
    return () => {
      cancelAnimationFrame(outer);
      cancelAnimationFrame(inner);
    };
  }, [result]);
  const failure = cleanup.isError ? errorDetailsOf(cleanup.error) : null;

  function runCleanup() {
    if (cleanup.isPending) return;
    cleanup.mutate(undefined, {
      onSuccess: (data) => {
        setResult(data);
        setOpen(false);
      },
    });
  }

  return (
    <div className="space-y-2 border-t border-border pt-4">
      <p className="text-sm font-medium">{t('backupTemp.cleanup.title')}</p>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="max-w-prose text-sm text-muted-foreground">{t('backupTemp.cleanup.description')}</p>
        <Tooltip>
          <TooltipTrigger asChild>
            <TooltipFocusTarget tabIndex={nothing ? 0 : -1} className="inline">
              <Button
                type="button"
                variant="destructive"
                size="sm"
                permission={PERM.BACKUPS_MANAGE}
                disabled={nothing}
                onClick={() => {
                  cleanup.reset();
                  setOpen(true);
                }}
              >
                <IconEraser className="size-4" />
                {t('backupTemp.cleanup.button')}
              </Button>
            </TooltipFocusTarget>
          </TooltipTrigger>
          {nothing && <TooltipContent>{t('backupTemp.cleanup.none')}</TooltipContent>}
        </Tooltip>
      </div>
      <output ref={outputRef} aria-live="polite" tabIndex={-1} className="block text-sm outline-none">
        {result ? resultText(result, (key, o) => t(key, o)) : null}
      </output>
      <ConfirmDialog
        open={isOpen}
        onOpenChange={setOpen}
        title={t('backupTemp.cleanup.confirmTitle')}
        description={
          <div className="space-y-2">
            <p>
              {t('backupTemp.cleanup.confirmBody', {
                entries: temp?.entries ?? 0,
                size: formatBytes(temp?.sizeBytes ?? 0),
                path: temp?.path ?? '',
              })}
            </p>
            {temp && temp.activeRuns > 0 && <p>{t('backupTemp.cleanup.confirmActive')}</p>}
            {failure && <Alert variant="destructive" title={failure.summary} description={failure.detail} />}
          </div>
        }
        confirmLabel={t('backupTemp.cleanup.button')}
        loading={cleanup.isPending}
        onConfirm={runCleanup}
      />
    </div>
  );
}
